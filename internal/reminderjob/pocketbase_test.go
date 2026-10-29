package reminderjob_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

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

	return app
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
