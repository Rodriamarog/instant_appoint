package reminderjob_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/reminderjob"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// graph is a stand-in for Meta's send endpoint. It records what arrived, so
// the assertions below are about the request a real Graph would have seen.
type graph struct {
	mu       sync.Mutex
	requests []map[string]any
	paths    []string
	refuse   bool
}

func (g *graph) start(t *testing.T) *meta.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		g.mu.Lock()
		g.requests = append(g.requests, body)
		g.paths = append(g.paths, r.URL.Path)
		refuse := g.refuse
		g.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if refuse {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Recipient phone number not in allowed list",` +
				`"type":"OAuthException","code":131030,"fbtrace_id":"Axb"}}`))

			return
		}

		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.HBgLNTIx"}]}`))
	}))
	t.Cleanup(server.Close)

	client := meta.New("META_APP_ID", "META_APP_SECRET")
	client.BaseURL = server.URL

	return client
}

func (g *graph) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return len(g.requests)
}

func TestOneDueReminderSendsExactlyOneTemplateAndWritesExactlyOneRow(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	connectNumber(t, app, alice)

	now := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)
	save(t, app, "reminder_settings", map[string]any{
		"user_id": alice, "timing_minutes": 30, "is_active": true,
		"template_name": "appointment_reminder", "template_language": "es", "timezone": "America/Tijuana",
	})
	eventID := save(t, app, "calendar_events", map[string]any{
		"user_id": alice, "title": "Ana Lopez", "client_phone": "5216641234567",
		"start_time": iso(now.Add(25 * time.Minute)), "end_time": iso(now.Add(55 * time.Minute)),
	})

	stub := &graph{}
	store := reminderjob.NewStore(app)
	sender := reminderjob.NewSender(stub.start(t))

	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Sent != 1 {
		t.Fatalf("summary = %+v, want one reminder sent", summary)
	}

	// The next three ticks must change nothing at all.
	for minute := 1; minute <= 3; minute++ {
		if _, err := reminders.Run(context.Background(), store, sender, now.Add(time.Duration(minute)*time.Minute)); err != nil {
			t.Fatalf("Run at +%dm: %v", minute, err)
		}
	}

	if stub.count() != 1 {
		t.Errorf("Meta was called %d times, want exactly 1", stub.count())
	}
	rows := reminderRows(t, app, eventID)
	if len(rows) != 1 {
		t.Fatalf("%d reminder rows were written, want exactly 1", len(rows))
	}

	row := rows[0]
	if row.GetString("message_id") != "wamid.HBgLNTIx" {
		t.Errorf("message_id = %q, want the id the stub returned", row.GetString("message_id"))
	}
	if row.GetString("direction") != "outbound" || row.GetString("conversation_id") == "" {
		t.Errorf("row = %v, want an outbound row attached to a conversation", row.FieldsData())
	}

	// And what Meta saw: the approved template, in the owner's language, with
	// the three positional parameters spelled in the owner's zone. 16:30 UTC
	// is 08:30 in Tijuana in November.
	request := stub.requests[0]
	if !strings.HasSuffix(stub.paths[0], "/v21.0/200000000000002/messages") {
		t.Errorf("Meta was called at %q, want the connected number's messages endpoint", stub.paths[0])
	}
	if request["type"] != "template" || request["to"] != "5216641234567" {
		t.Errorf("request = %v, want a template to the client", request)
	}

	template, _ := request["template"].(map[string]any)
	if template["name"] != "appointment_reminder" {
		t.Errorf("template name = %v, want appointment_reminder", template["name"])
	}
	if language, _ := template["language"].(map[string]any); language["code"] != "es" {
		t.Errorf("language = %v, want es", template["language"])
	}

	components, _ := template["components"].([]any)
	if len(components) != 1 {
		t.Fatalf("components = %v, want one body component", template["components"])
	}
	component, _ := components[0].(map[string]any)
	if component["type"] != "body" {
		t.Errorf("component type = %v, want lowercase body", component["type"])
	}

	parameters, _ := component["parameters"].([]any)
	want := []string{"Ana Lopez", "03/11/2026", "08:30"}
	if len(parameters) != len(want) {
		t.Fatalf("parameters = %v, want %v", parameters, want)
	}
	for i, value := range want {
		parameter, _ := parameters[i].(map[string]any)
		if parameter["type"] != "text" || parameter["text"] != value {
			t.Errorf("parameter {{%d}} = %v, want text %q", i+1, parameter, value)
		}
	}
}

// A reminder nobody chose a template for cannot be sent, and Meta is never
// called to find that out. template_name has no default and is empty until
// somebody picks one in the settings dialog.
func TestNoTemplateChosenMeansMetaIsNeverCalled(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	connectNumber(t, app, alice)

	now := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)
	save(t, app, "reminder_settings", map[string]any{
		"user_id": alice, "timing_minutes": 30, "is_active": true,
		"template_language": "es", "timezone": "America/Tijuana",
	})
	eventID := save(t, app, "calendar_events", map[string]any{
		"user_id": alice, "title": "Ana Lopez", "client_phone": "5216641234567",
		"start_time": iso(now.Add(25 * time.Minute)), "end_time": iso(now.Add(55 * time.Minute)),
	})

	stub := &graph{}
	summary, err := reminders.Run(context.Background(),
		reminderjob.NewStore(app), reminderjob.NewSender(stub.start(t)), now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if summary.NoTemplate != 1 {
		t.Errorf("summary = %+v, want the reminder skipped for having no template", summary)
	}
	if stub.count() != 0 {
		t.Errorf("Meta was called %d times for a template nobody has chosen", stub.count())
	}
	if rows := reminderRows(t, app, eventID); len(rows) != 0 {
		t.Errorf("%d rows were written although nothing was sent", len(rows))
	}
}

// A send Meta refuses every time is tried three times and then left alone, so
// a revoked token does not mean a Graph call every sixty seconds until the
// appointment starts.
func TestASendMetaAlwaysRefusesIsTriedThreeTimesAndThenStops(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	connectNumber(t, app, alice)

	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	save(t, app, "reminder_settings", map[string]any{
		"user_id": alice, "timing_minutes": 480, "is_active": true,
		"template_name": "appointment_reminder", "template_language": "es", "timezone": "America/Tijuana",
	})
	eventID := save(t, app, "calendar_events", map[string]any{
		"user_id": alice, "title": "Ana Lopez", "client_phone": "5216641234567",
		"start_time": iso(now.Add(7 * time.Hour)), "end_time": iso(now.Add(8 * time.Hour)),
	})

	stub := &graph{refuse: true}
	store := reminderjob.NewStore(app)
	sender := reminderjob.NewSender(stub.start(t))

	// Two hours of ticks, a minute apart.
	for minute := 0; minute < 120; minute++ {
		if _, err := reminders.Run(context.Background(), store, sender,
			now.Add(time.Duration(minute)*time.Minute)); err != nil {
			t.Fatalf("Run at +%dm: %v", minute, err)
		}
	}

	if stub.count() != reminders.MaxAttempts {
		t.Errorf("Meta was called %d times over two hours, want exactly %d", stub.count(), reminders.MaxAttempts)
	}

	rows := reminderRows(t, app, eventID)
	if len(rows) != reminders.MaxAttempts {
		t.Fatalf("%d rows were written, want one per attempt (%d)", len(rows), reminders.MaxAttempts)
	}
	for i, row := range rows {
		if row.GetString("status") != "failed" {
			t.Errorf("attempt %d has status %q, want failed", i+1, row.GetString("status"))
		}
		// The raw Graph response can carry the app secret and must never be
		// stored. Only the template and its parameters are.
		if strings.Contains(row.GetString("message_content"), "META_APP_SECRET") ||
			strings.Contains(row.GetString("message_content"), "fbtrace_id") {
			t.Errorf("the stored row carries the Graph response: %q", row.GetString("message_content"))
		}
	}
}

// A send that fails once and then succeeds goes out, and leaves one failed
// attempt beside the one that worked.
func TestASendThatRecoversGoesOut(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	connectNumber(t, app, alice)

	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	save(t, app, "reminder_settings", map[string]any{
		"user_id": alice, "timing_minutes": 480, "is_active": true,
		"template_name": "appointment_reminder", "template_language": "es", "timezone": "America/Tijuana",
	})
	eventID := save(t, app, "calendar_events", map[string]any{
		"user_id": alice, "title": "Ana Lopez", "client_phone": "5216641234567",
		"start_time": iso(now.Add(7 * time.Hour)), "end_time": iso(now.Add(8 * time.Hour)),
	})

	stub := &graph{refuse: true}
	store := reminderjob.NewStore(app)
	sender := reminderjob.NewSender(stub.start(t))

	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("the first run: %v", err)
	}

	stub.mu.Lock()
	stub.refuse = false
	stub.mu.Unlock()

	// A minute later the backoff has not elapsed, so nothing is tried.
	if _, err := reminders.Run(context.Background(), store, sender, now.Add(time.Minute)); err != nil {
		t.Fatalf("the second run: %v", err)
	}
	if stub.count() != 1 {
		t.Fatalf("Meta was called %d times inside the backoff, want 1", stub.count())
	}

	if _, err := reminders.Run(context.Background(), store, sender, now.Add(reminders.RetryBackoff)); err != nil {
		t.Fatalf("the retry run: %v", err)
	}

	rows := reminderRows(t, app, eventID)
	if len(rows) != 2 {
		t.Fatalf("%d rows, want the failed attempt and the one that worked", len(rows))
	}
	if rows[0].GetString("status") != "failed" || rows[1].GetString("status") != "sent" {
		t.Errorf("statuses = %q, %q, want failed then sent",
			rows[0].GetString("status"), rows[1].GetString("status"))
	}
	if rows[1].GetString("message_id") != "wamid.HBgLNTIx" {
		t.Errorf("the successful row carries message_id %q", rows[1].GetString("message_id"))
	}
}
