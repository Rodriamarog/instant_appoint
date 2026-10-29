package migrations_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// reminder_settings is written and read by the browser through PocketBase's
// own record API, so every check here goes over real HTTP with a real auth
// token. That is the layer the access rules are evaluated at, and the rules
// are the only thing keeping one person's reminder configuration away from
// another's.
//
// newAPI, newPerson and call live in 1793232002_calendar_events_test.go.

const reminderSettings = "reminder_settings"

// settings is the body the settings dialog sends.
func settings(owner string, minutes float64) map[string]any {
	return map[string]any{
		"user_id":           owner,
		"timing_minutes":    minutes,
		"is_active":         true,
		"template_name":     "appointment_reminder",
		"template_language": "es",
		"timezone":          "America/Tijuana",
	}
}

// saveSettings writes a row directly, bypassing the API, so a test can set up
// another person's data without leaning on the rules it is about to probe.
func saveSettings(t *testing.T, app core.App, owner string, minutes float64) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(reminderSettings)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(%s): %v", reminderSettings, err)
	}

	record := core.NewRecord(collection)
	for key, value := range settings(owner, minutes) {
		record.Set(key, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	return record.Id
}

func TestReminderSettingsStoreEveryFieldThePageSets(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")

	status, created := call(t, mux, alice.token, http.MethodPost,
		"/api/collections/reminder_settings/records", settings(alice.id, 120))
	if status != http.StatusOK {
		t.Fatalf("create: status %d, want 200 (body %v)", status, created)
	}

	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("create returned no id: %v", created)
	}

	status, read := call(t, mux, alice.token, http.MethodGet,
		"/api/collections/reminder_settings/records/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("view own settings: status %d, want 200 (body %v)", status, read)
	}

	want := map[string]any{
		"timing_minutes":    float64(120),
		"is_active":         true,
		"template_name":     "appointment_reminder",
		"template_language": "es",
		"timezone":          "America/Tijuana",
	}
	for field, value := range want {
		if read[field] != value {
			t.Errorf("%s = %#v, want %#v", field, read[field], value)
		}
	}
}

// A row written by anything that does not know about the two new fields — an
// older client, a script, the scheduler — still has to come back with a
// template language and a zone, because #9 formats a date with them.
func TestReminderSettingsAreBornWithALanguageAndAZone(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")

	status, created := call(t, mux, alice.token, http.MethodPost,
		"/api/collections/reminder_settings/records", map[string]any{
			"user_id":        alice.id,
			"timing_minutes": 60,
			"is_active":      true,
		})
	if status != http.StatusOK {
		t.Fatalf("create without the new fields: status %d, want 200 (body %v)", status, created)
	}

	if created["template_language"] != "es" {
		t.Errorf("template_language = %#v, want \"es\"", created["template_language"])
	}
	if created["timezone"] != "America/Tijuana" {
		t.Errorf("timezone = %#v, want \"America/Tijuana\"", created["timezone"])
	}
	// There is no template that is right for everybody, so this one stays
	// empty and #9 has to treat empty as "not chosen yet".
	if created["template_name"] != "" {
		t.Errorf("template_name = %#v, want it left empty", created["template_name"])
	}
}

// Every reader resolves settings with a first-match lookup. A second row for
// the same person would make which one wins arbitrary, so the database has to
// refuse it rather than the code having to cope.
func TestAPersonCannotEndUpWithTwoSetsOfSettings(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")

	status, first := call(t, mux, alice.token, http.MethodPost,
		"/api/collections/reminder_settings/records", settings(alice.id, 60))
	if status != http.StatusOK {
		t.Fatalf("first create: status %d, want 200 (body %v)", status, first)
	}

	status, second := call(t, mux, alice.token, http.MethodPost,
		"/api/collections/reminder_settings/records", settings(alice.id, 1440))
	if status == http.StatusOK {
		t.Fatalf("a second settings row was accepted: %v", second)
	}
	t.Logf("duplicate create: status %d, body %v", status, second)

	rows, err := app.FindAllRecords(reminderSettings)
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("%d settings rows exist, want 1", len(rows))
	}

	// Two different people each having their own row is the whole point, so
	// the index must not be unique on nothing in particular.
	bob := newPerson(t, app, "bob@instantappoint.test")
	status, bobs := call(t, mux, bob.token, http.MethodPost,
		"/api/collections/reminder_settings/records", settings(bob.id, 30))
	if status != http.StatusOK {
		t.Fatalf("Bob could not save his own settings: status %d (body %v)", status, bobs)
	}
}

// The acceptance criterion "a person sees only their own settings", put to
// the direct API call that attempts exactly what the rules must forbid, for
// every verb.
func TestAPersonCannotTouchSomeoneElsesReminderSettings(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")
	bob := newPerson(t, app, "bob@instantappoint.test")

	aliceSettings := saveSettings(t, app, alice.id, 1440)

	t.Run("list", func(t *testing.T) {
		saveSettings(t, app, bob.id, 30)

		status, listed := call(t, mux, bob.token, http.MethodGet,
			"/api/collections/reminder_settings/records?perPage=200", nil)
		if status != http.StatusOK {
			t.Fatalf("list: status %d, want 200 (body %v)", status, listed)
		}

		items, _ := listed["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("Bob sees %d settings rows, want 1 (body %v)", len(items), listed)
		}
		first, _ := items[0].(map[string]any)
		if first["user_id"] != bob.id {
			t.Errorf("Bob's list returned a row owned by %v", first["user_id"])
		}
	})

	t.Run("view", func(t *testing.T) {
		status, body := call(t, mux, bob.token, http.MethodGet,
			"/api/collections/reminder_settings/records/"+aliceSettings, nil)
		if status == http.StatusOK {
			t.Errorf("Bob read Alice's settings: %v", body)
		}
		t.Logf("cross-user view: status %d", status)
	})

	t.Run("create in someone else's name", func(t *testing.T) {
		status, body := call(t, mux, bob.token, http.MethodPost,
			"/api/collections/reminder_settings/records", settings(alice.id, 15))
		if status == http.StatusOK {
			t.Errorf("Bob created settings owned by Alice: %v", body)
		}
		t.Logf("create forgery: status %d", status)
	})

	t.Run("edit", func(t *testing.T) {
		status, body := call(t, mux, bob.token, http.MethodPatch,
			"/api/collections/reminder_settings/records/"+aliceSettings,
			map[string]any{"is_active": false})
		if status == http.StatusOK {
			t.Errorf("Bob switched Alice's reminders off: %v", body)
		}
		t.Logf("cross-user update: status %d", status)

		record, err := app.FindRecordById(reminderSettings, aliceSettings)
		if err != nil {
			t.Fatalf("FindRecordById: %v", err)
		}
		if !record.GetBool("is_active") {
			t.Error("Alice's reminders are switched off; Bob's edit landed")
		}
	})

	t.Run("delete", func(t *testing.T) {
		status, body := call(t, mux, bob.token, http.MethodDelete,
			"/api/collections/reminder_settings/records/"+aliceSettings, nil)
		if status == http.StatusOK || status == http.StatusNoContent {
			t.Errorf("Bob deleted Alice's settings: status %d, %v", status, body)
		}
		t.Logf("cross-user delete: status %d", status)

		if _, err := app.FindRecordById(reminderSettings, aliceSettings); err != nil {
			t.Errorf("Alice's settings are gone: %v", err)
		}
	})
}

func TestAnUnauthenticatedVisitorSeesNoReminderSettings(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")
	id := saveSettings(t, app, alice.id, 1440)

	status, listed := call(t, mux, "", http.MethodGet,
		"/api/collections/reminder_settings/records", nil)
	if status == http.StatusOK {
		if items, _ := listed["items"].([]any); len(items) != 0 {
			t.Errorf("a logged-out list returned %d rows, want none", len(items))
		}
	}

	status, _ = call(t, mux, "", http.MethodGet,
		"/api/collections/reminder_settings/records/"+id, nil)
	if status == http.StatusOK {
		t.Error("a logged-out visitor read someone's reminder settings")
	}

	status, body := call(t, mux, "", http.MethodPost,
		"/api/collections/reminder_settings/records", settings(alice.id, 15))
	if status == http.StatusOK {
		t.Errorf("a logged-out visitor created settings: %v", body)
	}
}

// The other half: rules tight enough to lock the owner out would pass every
// test above.
func TestTheOwnerCanChangeAndSwitchOffTheirOwnReminders(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")
	id := saveSettings(t, app, alice.id, 1440)

	status, body := call(t, mux, alice.token, http.MethodPatch,
		"/api/collections/reminder_settings/records/"+id,
		map[string]any{"timing_minutes": 120, "is_active": false})
	if status != http.StatusOK {
		t.Fatalf("owner update: status %d, want 200 (body %v)", status, body)
	}
	if body["timing_minutes"] != float64(120) || body["is_active"] != false {
		t.Errorf("update did not persist both fields: %v", body)
	}
}

// #9 divides the lookahead window by the lead time. A zero or negative one
// has no meaning there, so it is refused at the edge rather than guarded
// against in the scheduler.
func TestALeadTimeMustBeAWholeNumberOfMinutesInTheFuture(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")

	for _, minutes := range []float64{0, -30, 1.5} {
		status, body := call(t, mux, alice.token, http.MethodPost,
			"/api/collections/reminder_settings/records", settings(alice.id, minutes))
		if status == http.StatusOK {
			t.Errorf("timing_minutes %v was accepted: %v", minutes, body)
		}
	}
}
