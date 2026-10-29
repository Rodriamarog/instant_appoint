package migrations_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// The calendar_events access rules are the only thing standing between one
// user's appointments and another's: the SPA talks to PocketBase directly, so
// there is no server-side handler left to check ownership. Everything below
// goes through the real HTTP API — the same surface a browser or a curl
// command reaches — because that is the layer the rules are evaluated at.

// newAPI returns a bootstrapped app with every migration applied and the
// PocketBase HTTP API built on top of it, backed by a throwaway data dir.
func newAPI(t *testing.T) (core.App, http.Handler) {
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

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("apis.NewRouter: %v", err)
	}
	mux, err := pbRouter.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	return app, mux
}

// person is a real user row plus the auth token its browser would hold.
type person struct {
	id    string
	token string
}

func newPerson(t *testing.T, app core.App, email string) person {
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

	token, err := record.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken for %s: %v", email, err)
	}

	return person{id: record.Id, token: token}
}

// call issues an authenticated request against the record API and returns the
// status and the decoded body.
func call(t *testing.T, mux http.Handler, token, method, url string, body any) (int, map[string]any) {
	t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

	req := httptest.NewRequest(method, url, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	decoded := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}

	return rec.Code, decoded
}

// appointment is the body the SPA sends, minus the owner.
func appointment(owner string, title string) map[string]any {
	return map[string]any{
		"user_id":      owner,
		"title":        title,
		"client_phone": "16197612314",
		"notes":        "Hello, just a reminder about your appointment tomorrow.",
		"start_time":   "2026-11-02T17:00:00.000Z",
		"end_time":     "2026-11-02T18:00:00.000Z",
		"location":     "Suite 2",
	}
}

// saveAppointment writes a row directly, bypassing the API, so a test can set
// up another person's data without relying on the rules it is about to probe.
func saveAppointment(t *testing.T, app core.App, owner, title string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("calendar_events")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(calendar_events): %v", err)
	}

	record := core.NewRecord(collection)
	for key, value := range appointment(owner, title) {
		record.Set(key, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatalf("save appointment: %v", err)
	}

	return record.Id
}

func TestAnAppointmentIsCreatedAndReadBackByItsOwner(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")

	status, created := call(t, mux, alice.token, http.MethodPost,
		"/api/collections/calendar_events/records", appointment(alice.id, "Alice's haircut"))
	if status != http.StatusOK {
		t.Fatalf("create: status %d, want 200 (body %v)", status, created)
	}

	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("create returned no id: %v", created)
	}

	status, read := call(t, mux, alice.token, http.MethodGet,
		"/api/collections/calendar_events/records/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("view own record: status %d, want 200 (body %v)", status, read)
	}
	for _, field := range []string{"title", "client_phone", "notes", "start_time", "end_time", "location"} {
		if _, ok := read[field]; !ok {
			t.Errorf("field %q is missing from the stored record; the dialog writes it", field)
		}
	}
	if read["location"] != "Suite 2" {
		t.Errorf("location = %v, want %q", read["location"], "Suite 2")
	}
}

func TestListingShowsOnlyYourOwnAppointments(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")
	bob := newPerson(t, app, "bob@instantappoint.test")

	saveAppointment(t, app, alice.id, "Alice's haircut")
	saveAppointment(t, app, bob.id, "Bob's root canal")

	status, listed := call(t, mux, alice.token, http.MethodGet,
		"/api/collections/calendar_events/records?perPage=200", nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, want 200 (body %v)", status, listed)
	}

	items, _ := listed["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("Alice sees %d appointments, want 1 (body %v)", len(items), listed)
	}
	first, _ := items[0].(map[string]any)
	if first["title"] != "Alice's haircut" {
		t.Errorf("Alice sees %v, want her own appointment", first["title"])
	}
}

func TestAnUnauthenticatedVisitorSeesNothingAndCanWriteNothing(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")
	id := saveAppointment(t, app, alice.id, "Alice's haircut")

	status, listed := call(t, mux, "", http.MethodGet,
		"/api/collections/calendar_events/records", nil)
	if status == http.StatusOK {
		if items, _ := listed["items"].([]any); len(items) != 0 {
			t.Errorf("a logged-out list returned %d records, want none", len(items))
		}
	}

	status, _ = call(t, mux, "", http.MethodGet,
		"/api/collections/calendar_events/records/"+id, nil)
	if status == http.StatusOK {
		t.Error("a logged-out visitor could view an appointment")
	}

	status, body := call(t, mux, "", http.MethodPost,
		"/api/collections/calendar_events/records", appointment(alice.id, "Forged by nobody"))
	if status == http.StatusOK {
		t.Errorf("a logged-out visitor created an appointment: %v", body)
	}
}

// TestForgery is the acceptance criterion: a direct API call that attempts
// exactly the thing the rules must forbid, for every verb that writes.
func TestAPersonCannotTouchSomeoneElsesAppointment(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")
	bob := newPerson(t, app, "bob@instantappoint.test")

	aliceEvent := saveAppointment(t, app, alice.id, "Alice's haircut")

	t.Run("create in someone else's name", func(t *testing.T) {
		before := countEvents(t, app)

		status, body := call(t, mux, bob.token, http.MethodPost,
			"/api/collections/calendar_events/records", appointment(alice.id, "Forged by Bob"))

		if status == http.StatusOK {
			t.Errorf("Bob created an appointment owned by Alice: %v", body)
		}
		if got := countEvents(t, app); got != before {
			t.Errorf("row count went %d -> %d; the forged row landed", before, got)
		}
		t.Logf("create forgery: status %d", status)
	})

	t.Run("read someone else's appointment", func(t *testing.T) {
		status, body := call(t, mux, bob.token, http.MethodGet,
			"/api/collections/calendar_events/records/"+aliceEvent, nil)
		if status == http.StatusOK {
			t.Errorf("Bob viewed Alice's appointment: %v", body)
		}
		t.Logf("cross-user view: status %d", status)
	})

	t.Run("edit someone else's appointment", func(t *testing.T) {
		status, body := call(t, mux, bob.token, http.MethodPatch,
			"/api/collections/calendar_events/records/"+aliceEvent,
			map[string]any{"title": "Bob was here"})
		if status == http.StatusOK {
			t.Errorf("Bob edited Alice's appointment: %v", body)
		}
		t.Logf("cross-user update: status %d", status)

		record, err := app.FindRecordById("calendar_events", aliceEvent)
		if err != nil {
			t.Fatalf("FindRecordById: %v", err)
		}
		if got := record.GetString("title"); got != "Alice's haircut" {
			t.Errorf("title is now %q; Bob's edit landed", got)
		}
	})

	t.Run("steal someone else's appointment", func(t *testing.T) {
		// The other half of update: not changing the contents, but
		// reassigning the owner so it shows up on Bob's calendar.
		status, body := call(t, mux, bob.token, http.MethodPatch,
			"/api/collections/calendar_events/records/"+aliceEvent,
			map[string]any{"user_id": bob.id})
		if status == http.StatusOK {
			t.Errorf("Bob reassigned Alice's appointment to himself: %v", body)
		}
		t.Logf("cross-user ownership change: status %d", status)
	})

	t.Run("delete someone else's appointment", func(t *testing.T) {
		status, body := call(t, mux, bob.token, http.MethodDelete,
			"/api/collections/calendar_events/records/"+aliceEvent, nil)
		if status == http.StatusOK || status == http.StatusNoContent {
			t.Errorf("Bob deleted Alice's appointment: status %d, %v", status, body)
		}
		t.Logf("cross-user delete: status %d", status)

		if _, err := app.FindRecordById("calendar_events", aliceEvent); err != nil {
			t.Errorf("Alice's appointment is gone: %v", err)
		}
	})
}

// TestOwnerCanStillEditAndDelete guards against the obvious over-correction:
// rules so tight that the owner is locked out too.
func TestTheOwnerCanStillEditAndDeleteTheirOwnAppointment(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")
	id := saveAppointment(t, app, alice.id, "Alice's haircut")

	status, body := call(t, mux, alice.token, http.MethodPatch,
		"/api/collections/calendar_events/records/"+id,
		map[string]any{"title": "Alice's trim", "location": "Suite 3"})
	if status != http.StatusOK {
		t.Fatalf("owner update: status %d, want 200 (body %v)", status, body)
	}
	if body["title"] != "Alice's trim" || body["location"] != "Suite 3" {
		t.Errorf("update did not persist both fields: %v", body)
	}

	status, body = call(t, mux, alice.token, http.MethodDelete,
		"/api/collections/calendar_events/records/"+id, nil)
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("owner delete: status %d, want 204 (body %v)", status, body)
	}
	if _, err := app.FindRecordById("calendar_events", id); err == nil {
		t.Error("the record survived its owner's delete")
	}
}

// TestRangeFilter is the query the calendar page runs on every navigation.
// It is a lexicographic string comparison over a text column, which is why the
// millisecond-precision format is load-bearing.
func TestTheMonthWindowFilterSelectsOverlappingAppointments(t *testing.T) {
	app, mux := newAPI(t)
	alice := newPerson(t, app, "alice@instantappoint.test")

	collection, err := app.FindCollectionByNameOrId("calendar_events")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}
	for _, slot := range []struct{ title, start, end string }{
		{"last month", "2026-10-05T17:00:00.000Z", "2026-10-05T18:00:00.000Z"},
		{"this month", "2026-11-12T17:00:00.000Z", "2026-11-12T18:00:00.000Z"},
		{"next month", "2026-12-20T17:00:00.000Z", "2026-12-20T18:00:00.000Z"},
	} {
		record := core.NewRecord(collection)
		record.Set("user_id", alice.id)
		record.Set("title", slot.title)
		record.Set("start_time", slot.start)
		record.Set("end_time", slot.end)
		if err := app.Save(record); err != nil {
			t.Fatalf("save %s: %v", slot.title, err)
		}
	}

	filter := `end_time >= "2026-10-25T00:00:00.000Z" && start_time <= "2026-12-07T00:00:00.000Z"`
	status, listed := call(t, mux, alice.token, http.MethodGet,
		"/api/collections/calendar_events/records?perPage=200&sort=start_time&filter="+url.QueryEscape(filter), nil)
	if status != http.StatusOK {
		t.Fatalf("filtered list: status %d, want 200 (body %v)", status, listed)
	}

	items, _ := listed["items"].([]any)
	var titles []string
	for _, item := range items {
		m, _ := item.(map[string]any)
		titles = append(titles, fmt.Sprint(m["title"]))
	}

	want := []string{"this month"}
	if len(titles) != len(want) || titles[0] != want[0] {
		t.Errorf("window returned %v, want %v", titles, want)
	}
}

func countEvents(t *testing.T, app core.App) int {
	t.Helper()

	records, err := app.FindAllRecords("calendar_events")
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}

	return len(records)
}
