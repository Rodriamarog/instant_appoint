package migrations_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/migrations"
)

// Reading a customer's thread is the browser talking to PocketBase directly —
// there is no handler in front of these two collections — so their access
// rules are the only thing keeping one business's customers away from
// another's. Everything below goes through the real record API, with the same
// filters and sorts web/src/lib/conversations.ts and messages.ts send,
// because that is the layer the rules are evaluated at.
//
// These tests arrived with the chat page's send route and were kept when #21
// deleted it. What they cover is the storage the webhook writes and the
// assistant reads, which outlived the page that was the first thing to read
// it.

// listRecords issues one list request and returns the items.
func listRecords(t *testing.T, mux http.Handler, token, collection, filter, sort string) []map[string]any {
	t.Helper()

	url := "/api/collections/" + collection + "/records?perPage=200&sort=" + sort
	if filter != "" {
		url += "&filter=" + filter
	}

	status, body := call(t, mux, token, http.MethodGet, url, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200 (body: %v)", collection, status, body)
	}

	raw, _ := body["items"].([]any)
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, _ := item.(map[string]any)
		items = append(items, row)
	}

	return items
}

// saveConversation writes a customer thread directly, the way a delivery
// would have left one.
func saveConversation(t *testing.T, app core.App, owner, customerPhone, lastMessageAt string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(migrations.ConversationsCollection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(conversations): %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", owner)
	record.Set("customer_phone", customerPhone)
	record.Set("status", "active")
	record.Set("phone_number_id", "200000000000002")
	record.Set("last_message_at", lastMessageAt)
	if err := app.Save(record); err != nil {
		t.Fatalf("Save conversation: %v", err)
	}

	return record
}

// saveMessage writes one message into a conversation directly.
func saveMessage(t *testing.T, app core.App, conv *core.Record, direction, content, sentAt string) {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(migrations.WhatsAppMessagesCollection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(whatsapp_messages): %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", conv.GetString("user_id"))
	record.Set("to_number", conv.GetString("customer_phone"))
	record.Set("message_content", content)
	record.Set("message_type", "text")
	record.Set("direction", direction)
	record.Set("conversation_id", conv.Id)
	record.Set("sent_at", sentAt)
	if err := app.Save(record); err != nil {
		t.Fatalf("Save message: %v", err)
	}
}

// The conversation list is this person's own, most recently active first.
// last_message_at is a text column, so "most recent first" is a descending
// string sort and only holds while every writer emits the same fixed width.
func TestTheConversationListIsYourOwnMostRecentFirst(t *testing.T) {
	app, mux := newAPI(t)
	owner := newPerson(t, app, "owner@example.test")
	stranger := newPerson(t, app, "stranger@example.test")

	saveConversation(t, app, owner.id, "5216641111111", "2026-10-28T09:00:00.000Z")
	saveConversation(t, app, owner.id, "5216642222222", "2026-10-29T18:30:05.250Z")
	saveConversation(t, app, owner.id, "5216643333333", "2026-10-29T18:30:05.025Z")
	saveConversation(t, app, stranger.id, "5216649999999", "2026-10-30T09:00:00.000Z")

	items := listRecords(t, mux, owner.token, migrations.ConversationsCollection, "", "-last_message_at")

	if len(items) != 3 {
		t.Fatalf("listed %d conversations, want the owner's 3: %v", len(items), items)
	}

	want := []string{"5216642222222", "5216643333333", "5216641111111"}
	for i, phone := range want {
		if items[i]["customer_phone"] != phone {
			t.Errorf("conversation %d is %v, want %s", i, items[i]["customer_phone"], phone)
		}
	}
}

// The rule, not the filter, is what scopes the list — so a request that asks
// for someone else's conversations still only gets its own.
func TestAPersonCannotReadAnotherPersonsConversations(t *testing.T) {
	app, mux := newAPI(t)
	owner := newPerson(t, app, "owner@example.test")
	stranger := newPerson(t, app, "stranger@example.test")

	conv := saveConversation(t, app, owner.id, "5216641234567", "2026-10-29T12:00:00.000Z")
	saveMessage(t, app, conv, "inbound", "Can I move my appointment?", "2026-10-29T12:00:00.000Z")

	// Asking for them by owner id.
	byOwner := listRecords(t, mux, stranger.token, migrations.ConversationsCollection,
		`user_id="`+owner.id+`"`, "-last_message_at")
	if len(byOwner) != 0 {
		t.Errorf("a stranger filtering by the owner's id read %d conversations: %v", len(byOwner), byOwner)
	}

	// Asking for the thread by the conversation id.
	byThread := listRecords(t, mux, stranger.token, migrations.WhatsAppMessagesCollection,
		`conversation_id="`+conv.Id+`"`, "sent_at")
	if len(byThread) != 0 {
		t.Errorf("a stranger read %d messages from another person's thread: %v", len(byThread), byThread)
	}

	// Asking for the conversation itself, by id.
	status, body := call(t, mux, stranger.token, http.MethodGet,
		"/api/collections/"+migrations.ConversationsCollection+"/records/"+conv.Id, nil)
	if status != http.StatusNotFound {
		t.Errorf("viewing another person's conversation: status = %d, want 404 (body: %v)", status, body)
	}

	// And an unauthenticated visitor sees nothing either.
	status, body = call(t, mux, "", http.MethodGet,
		"/api/collections/"+migrations.ConversationsCollection+"/records", nil)
	if items, ok := body["items"].([]any); status == http.StatusOK && ok && len(items) != 0 {
		t.Errorf("an anonymous visitor read %d conversations", len(items))
	}
}

// A thread reads oldest first, which is how a transcript is read and the order
// the AI is given its history in. sent_at is text too, so the same
// string-sorting rule applies, including across the millisecond.
func TestAThreadReadsOldestFirst(t *testing.T) {
	app, mux := newAPI(t)
	owner := newPerson(t, app, "owner@example.test")

	conv := saveConversation(t, app, owner.id, "5216641234567", "2026-10-29T12:00:00.000Z")
	other := saveConversation(t, app, owner.id, "5216649999999", "2026-10-29T12:00:00.000Z")

	saveMessage(t, app, conv, "outbound", "third", "2026-10-29T12:00:01.100Z")
	saveMessage(t, app, conv, "inbound", "first", "2026-10-29T12:00:00.090Z")
	saveMessage(t, app, conv, "outbound", "second", "2026-10-29T12:00:01.000Z")
	saveMessage(t, app, other, "inbound", "a different customer entirely", "2026-10-29T12:00:00.500Z")

	items := listRecords(t, mux, owner.token, migrations.WhatsAppMessagesCollection,
		`conversation_id="`+conv.Id+`"`, "sent_at")

	if len(items) != 3 {
		t.Fatalf("read %d messages, want this conversation's 3: %v", len(items), items)
	}
	for i, want := range []string{"first", "second", "third"} {
		if items[i]["message_content"] != want {
			t.Errorf("message %d is %v, want %q", i, items[i]["message_content"], want)
		}
	}
	if items[0]["direction"] != "inbound" || items[1]["direction"] != "outbound" {
		t.Errorf("directions = %v / %v, want the stored inbound and outbound", items[0]["direction"], items[1]["direction"])
	}
}

// The browser cannot write a conversation or a message. Every row in both
// collections is put there by the webhook, server-side, and that is what makes
// the stored history the record of what WhatsApp actually carried rather than
// of what a page claimed.
func TestTheBrowserCannotWriteAThreadItself(t *testing.T) {
	app, mux := newAPI(t)
	owner := newPerson(t, app, "owner@example.test")

	conv := saveConversation(t, app, owner.id, "5216641234567", "2026-10-29T12:00:00.000Z")

	for _, write := range []struct {
		name, method, path string
		body               map[string]any
	}{
		{
			"create a message", http.MethodPost,
			"/api/collections/" + migrations.WhatsAppMessagesCollection + "/records",
			map[string]any{
				"user_id": owner.id, "to_number": "5216641234567",
				"message_content": "I never sent this", "direction": "outbound",
				"conversation_id": conv.Id, "sent_at": "2026-10-29T13:00:00.000Z",
			},
		},
		{
			"create a conversation", http.MethodPost,
			"/api/collections/" + migrations.ConversationsCollection + "/records",
			map[string]any{"user_id": owner.id, "customer_phone": "5216640000000", "status": "active"},
		},
		{
			"edit my own conversation", http.MethodPatch,
			"/api/collections/" + migrations.ConversationsCollection + "/records/" + conv.Id,
			map[string]any{"customer_phone": "5216640000001"},
		},
	} {
		t.Run(write.name, func(t *testing.T) {
			status, body := call(t, mux, owner.token, write.method, write.path, write.body)

			if status != http.StatusForbidden && status != http.StatusNotFound {
				t.Errorf("status = %d, want the write refused (body: %v)", status, body)
			}
		})
	}
}
