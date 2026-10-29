package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// save stores one record of a collection built from the given fields,
// returning whatever PocketBase made of it.
func save(t *testing.T, app core.App, collectionName string, fields map[string]any) error {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(%q): %v", collectionName, err)
	}

	record := core.NewRecord(collection)
	for name, value := range fields {
		record.Set(name, value)
	}

	return app.Save(record)
}

// A field the schema does not declare makes every write that carries it fail
// with an "unknown field" validation error. Two of these — phone_number_id and
// whatsapp_account_id — were missing from the migrations this schema was
// recovered from and existed only because somebody added them by hand in the
// admin UI, which is how a fresh database ended up unable to store an inbound
// message at all.
func TestConversationsDeclaresEveryFieldTheCodeUses(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("conversations")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	for _, name := range []string{"user_id", "customer_phone", "status", "last_message_at", "phone_number_id"} {
		if collection.Fields.GetByName(name) == nil {
			t.Errorf("field %q is missing", name)
		}
	}
}

func TestWhatsAppMessagesDeclaresEveryFieldTheCodeUses(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("whatsapp_messages")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	want := []string{
		"user_id", "event_id", "to_number", "message_content", "message_type",
		"status", "sent_at", "message_id", "direction", "conversation_id",
		"whatsapp_account_id",
	}
	for _, name := range want {
		if collection.Fields.GetByName(name) == nil {
			t.Errorf("field %q is missing", name)
		}
	}
}

// Both collections hold one business's customer conversations, and the SPA
// queries them with the user's own token. Reads are owner-scoped; writes all
// happen in Go, which bypasses rules, so no client needs them.
//
// A nil write rule is not the same as an unset one: it is the explicit
// "superusers and server-side Go only" that makes the endpoint 403 for
// everyone else. The legacy schema left all five rules open, which let any
// signed-in user read and rewrite every other business's conversations.
func TestConversationRulesAreOwnerReadServerWrite(t *testing.T) {
	app := newApp(t)

	for _, name := range []string{"conversations", "whatsapp_messages"} {
		t.Run(name, func(t *testing.T) {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				t.Fatalf("FindCollectionByNameOrId: %v", err)
			}

			owner := "user_id = @request.auth.id"
			for label, rule := range map[string]*string{"list": collection.ListRule, "view": collection.ViewRule} {
				if rule == nil || *rule != owner {
					t.Errorf("%s rule = %v, want %q", label, rule, owner)
				}
			}
			for label, rule := range map[string]*string{
				"create": collection.CreateRule,
				"update": collection.UpdateRule,
				"delete": collection.DeleteRule,
			} {
				if rule != nil {
					t.Errorf("%s rule = %q, want nil (superuser and server-side Go only)", label, *rule)
				}
			}
		})
	}
}

// The deduplication key. Two deliveries of one WhatsApp message can be in
// flight at once, on two servers if need be, and only the database sees both.
// Without this index the second write succeeds and the customer is answered
// twice.
func TestAMessageIDCanOnlyBeStoredOnce(t *testing.T) {
	app := newApp(t)

	message := map[string]any{
		"user_id":    "user000000000001",
		"to_number":  "5215511111111",
		"message_id": "wamid.HBgNNTIx",
		"direction":  "inbound",
	}
	if err := save(t, app, "whatsapp_messages", message); err != nil {
		t.Fatalf("the first message was rejected: %v", err)
	}
	if err := save(t, app, "whatsapp_messages", message); err == nil {
		t.Error("the same message id was stored twice")
	}
}

// Rows with no WhatsApp id must not collide with each other, which is why the
// index is predicated on message_id != ”. A reminder that failed before Meta
// answered has no id, and so does any row written by a path that does not go
// through a send.
func TestMessagesWithNoMessageIDDoNotCollide(t *testing.T) {
	app := newApp(t)

	for i := 0; i < 2; i++ {
		err := save(t, app, "whatsapp_messages", map[string]any{
			"user_id":    "user000000000001",
			"to_number":  "5215511111111",
			"message_id": "",
			"direction":  "outbound",
		})
		if err != nil {
			t.Fatalf("message %d with no id was rejected: %v", i+1, err)
		}
	}
}

// What makes a reminder send-once across restarts and across two servers
// racing the same scheduled minute: the row recording the send is itself the
// lock. Ticket #9 relies on this.
func TestAReminderCanOnlySendOncePerEvent(t *testing.T) {
	app := newApp(t)

	reminder := map[string]any{
		"user_id":      "user000000000001",
		"to_number":    "5215511111111",
		"event_id":     "event00000000001",
		"message_type": "reminder",
		"status":       "sent",
	}
	if err := save(t, app, "whatsapp_messages", reminder); err != nil {
		t.Fatalf("the first reminder was rejected: %v", err)
	}
	if err := save(t, app, "whatsapp_messages", reminder); err == nil {
		t.Error("the same event was recorded as reminded twice")
	}
}

// The index is predicated on the message being a sent reminder, so the
// ordinary messages of an appointment's conversation are unaffected by it, and
// a reminder that failed can be retried.
func TestTheReminderIndexOnlyConstrainsSentReminders(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows map[string]any
	}{
		{"two inbound messages about one event", map[string]any{"message_type": "text", "status": "", "direction": "inbound"}},
		{"two failed reminders for one event", map[string]any{"message_type": "reminder", "status": "failed"}},
		{"two pending reminders for one event", map[string]any{"message_type": "reminder", "status": "pending"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newApp(t)

			for i := 0; i < 2; i++ {
				row := map[string]any{
					"user_id":   "user000000000001",
					"to_number": "5215511111111",
					"event_id":  "event00000000001",
				}
				for name, value := range tc.rows {
					row[name] = value
				}
				if err := save(t, app, "whatsapp_messages", row); err != nil {
					t.Fatalf("row %d was rejected: %v", i+1, err)
				}
			}
		})
	}
}

// A customer has one active conversation, and two messages arriving together
// must not each create one — the dashboard would then show the same customer
// twice, each holding half the conversation, and the AI would see half the
// history.
func TestACustomerHasOneActiveConversation(t *testing.T) {
	app := newApp(t)

	conversation := map[string]any{
		"user_id":        "user000000000001",
		"customer_phone": "5215511111111",
		"status":         "active",
	}
	if err := save(t, app, "conversations", conversation); err != nil {
		t.Fatalf("the first conversation was rejected: %v", err)
	}
	if err := save(t, app, "conversations", conversation); err == nil {
		t.Error("a second active conversation was created for the same customer")
	}
}

// Closing a conversation must not block the next one, so the status is part of
// the key rather than a filter on it.
func TestAClosedConversationDoesNotBlockANewOne(t *testing.T) {
	app := newApp(t)

	base := map[string]any{"user_id": "user000000000001", "customer_phone": "5215511111111"}

	closed := map[string]any{"status": "closed"}
	for name, value := range base {
		closed[name] = value
	}
	if err := save(t, app, "conversations", closed); err != nil {
		t.Fatalf("the closed conversation was rejected: %v", err)
	}

	active := map[string]any{"status": "active"}
	for name, value := range base {
		active[name] = value
	}
	if err := save(t, app, "conversations", active); err != nil {
		t.Errorf("a new active conversation was rejected: %v", err)
	}
}

// A message body is rejected rather than truncated when it exceeds a declared
// maximum, and rejecting an inbound message loses it. WhatsApp allows 4096
// characters and Meta has raised that before.
func TestALongMessageIsStored(t *testing.T) {
	app := newApp(t)

	body := make([]byte, 8000)
	for i := range body {
		body[i] = 'a'
	}

	err := save(t, app, "whatsapp_messages", map[string]any{
		"user_id":         "user000000000001",
		"to_number":       "5215511111111",
		"message_id":      "wamid.long",
		"direction":       "inbound",
		"message_content": string(body),
	})
	if err != nil {
		t.Errorf("an 8000-character message was rejected: %v", err)
	}
}
