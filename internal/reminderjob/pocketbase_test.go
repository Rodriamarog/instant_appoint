package reminderjob_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// Everything in this file stands a real PocketBase up against a throwaway
// data directory. The claim that makes a reminder go out exactly once is a
// unique index, and an index is not something a fake can be asked about: the
// double insert below has to be a genuine one.

// newApp returns a bootstrapped app with every migration applied.
func newApp(t *testing.T) core.App {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		if err := app.ResetBootstrapState(); err != nil {
			t.Errorf("ResetBootstrapState: %v", err)
		}
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	ensureMessageCollections(t, app)

	return app
}

// ensureMessageCollections creates conversations and whatsapp_messages when
// the migrations have not.
//
// They belong to ticket #6, which is being built in parallel, and declaring
// them in a migration here would be a guaranteed collision on the schema. So
// this stands them up in the test instead, with the fields and — crucially —
// the indexes this package depends on. Once #6 has landed the migration makes
// them and this is a no-op.
func ensureMessageCollections(t *testing.T, app core.App) {
	t.Helper()

	if _, err := app.FindCollectionByNameOrId("conversations"); err != nil {
		conversations := core.NewBaseCollection("conversations", "pbc_conversations")
		conversations.Fields.Add(
			&core.TextField{Name: "user_id", Required: true, Max: 255},
			&core.TextField{Name: "customer_phone", Required: true, Max: 50},
			&core.TextField{Name: "status", Max: 50},
			&core.TextField{Name: "last_message_at", Max: 50},
			&core.TextField{Name: "phone_number_id", Max: 255},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		conversations.Indexes = types.JSONArray[string]{
			"CREATE UNIQUE INDEX `idx_conversations_customer` ON `conversations` (`user_id`, `customer_phone`, `status`)",
		}
		if err := app.Save(conversations); err != nil {
			t.Fatalf("stand up conversations: %v", err)
		}
	}

	if _, err := app.FindCollectionByNameOrId("whatsapp_messages"); err != nil {
		messages := core.NewBaseCollection("whatsapp_messages", "pbc_whatsapp_messages")
		messages.Fields.Add(
			&core.TextField{Name: "user_id", Required: true, Max: 255},
			&core.TextField{Name: "to_number", Required: true, Max: 50},
			&core.TextField{Name: "message_content", Max: 65535},
			&core.TextField{Name: "message_type", Max: 50},
			&core.TextField{Name: "direction", Max: 20},
			&core.TextField{Name: "conversation_id", Max: 255},
			&core.TextField{Name: "message_id", Max: 255},
			&core.TextField{Name: "event_id", Max: 255},
			&core.TextField{Name: "status", Max: 50},
			&core.TextField{Name: "sent_at", Max: 50},
			&core.TextField{Name: "whatsapp_account_id", Max: 255},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		messages.Indexes = types.JSONArray[string]{
			"CREATE UNIQUE INDEX `idx_whatsapp_messages_message_id` ON `whatsapp_messages` (`message_id`) " +
				"WHERE `message_id` != ''",
			"CREATE UNIQUE INDEX `idx_whatsapp_messages_reminder_sent` ON `whatsapp_messages` (`event_id`) " +
				"WHERE `message_type` = 'reminder' AND `status` = 'sent'",
		}
		if err := app.Save(messages); err != nil {
			t.Fatalf("stand up whatsapp_messages: %v", err)
		}
	}
}

// save writes one record of a collection and returns its id.
func save(t *testing.T, app core.App, collection string, values map[string]any) string {
	t.Helper()

	model, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(%s): %v", collection, err)
	}

	record := core.NewRecord(model)
	for key, value := range values {
		record.Set(key, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatalf("save into %s: %v", collection, err)
	}

	return record.Id
}

// newUser writes a verified account and returns its id.
func newUser(t *testing.T, app core.App, email string) string {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(users): %v", err)
	}

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword("devpassword123")
	record.SetVerified(true)
	if err := app.Save(record); err != nil {
		t.Fatalf("save %s: %v", email, err)
	}

	return record.Id
}

// connectNumber gives a person an active Cloud API account.
func connectNumber(t *testing.T, app core.App, userID string) string {
	t.Helper()

	return save(t, app, "whatsapp_accounts", map[string]any{
		"user_id":         userID,
		"session_id":      "business_" + userID,
		"account_type":    "business_api",
		"waba_id":         "100000000000001",
		"phone_number_id": "200000000000002",
		"access_token":    "FAKE-SEEDED-TOKEN-not-a-real-meta-credential",
		"status":          "connected",
		"is_active":       true,
	})
}

// reminderRows returns every reminder row for an appointment.
func reminderRows(t *testing.T, app core.App, eventID string) []*core.Record {
	t.Helper()

	records, err := app.FindRecordsByFilter("whatsapp_messages",
		"event_id = {:eventID} && message_type = 'reminder'", "created", 0, 0,
		dbx.Params{"eventID": eventID})
	if err != nil {
		t.Fatalf("read the reminder rows for %s: %v", eventID, err)
	}

	return records
}

// iso renders an instant the way every stored timestamp in this app is
// written.
var iso = isotime.Format
