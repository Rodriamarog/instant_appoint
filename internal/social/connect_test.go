package social_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/social"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

func newApp(t *testing.T) core.App {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir(), HideStartBanner: true})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	return app
}

// graph answers the Graph calls the connection makes. pages is the
// me/accounts body; every call is recorded as METHOD path.
type graph struct {
	server *httptest.Server

	mu        sync.Mutex
	calls     []string
	pages     string
	subscribe string
}

func newGraph(t *testing.T, pages string) *graph {
	t.Helper()

	g := &graph{pages: pages, subscribe: `{"success":true}`}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.calls = append(g.calls, r.Method+" "+r.URL.Path)
		g.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth/access_token") && r.URL.Query().Get("code") != "":
			_, _ = w.Write([]byte(`{"access_token":"SHORT"}`))
		case strings.HasSuffix(r.URL.Path, "/oauth/access_token"):
			_, _ = w.Write([]byte(`{"access_token":"LONG"}`))
		case strings.HasSuffix(r.URL.Path, "/me/accounts"):
			_, _ = w.Write([]byte(g.pages))
		case strings.HasSuffix(r.URL.Path, "/subscribed_apps"):
			_, _ = w.Write([]byte(g.subscribe))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"unexpected"}}`))
		}
	}))
	t.Cleanup(g.server.Close)

	return g
}

func (g *graph) client() *meta.Client {
	c := meta.New("APP", "SECRET")
	c.BaseURL = g.server.URL
	return c
}

func (g *graph) saw(call string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, c := range g.calls {
		if strings.HasSuffix(c, call) && strings.HasPrefix(c, strings.Fields(call)[0]) {
			return true
		}
	}

	return false
}

const onePageWithInstagram = `{"data":[{"id":"P1","name":"Clínica Sol","access_token":"PT1",
	"instagram_business_account":{"id":"IG1","username":"clinicasol","name":"Clínica Sol IG"}}]}`

const twoPages = `{"data":[
	{"id":"P1","name":"Clínica Sol","access_token":"PT1","instagram_business_account":{"id":"IG1","username":"clinicasol"}},
	{"id":"P2","name":"Clínica Luna","access_token":"PT2"}]}`

func rows(t *testing.T, app core.App, userID string) map[string]*core.Record {
	t.Helper()

	records, err := app.FindRecordsByFilter(migrations.SocialAccountsCollection, "user_id = {:u}", "", 0, 0, dbx.Params{"u": userID})
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}

	out := map[string]*core.Record{}
	for _, record := range records {
		out[record.GetString("channel")] = record
	}

	return out
}

func TestOnePageConnectsMessengerAndItsInstagramAtOnce(t *testing.T) {
	app := newApp(t)
	g := newGraph(t, onePageWithInstagram)

	connected, choices, err := social.NewConnector(app, g.client()).Start(context.Background(), "clinic-1", "CODE")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if choices != nil {
		t.Fatalf("one Page was offered as a choice: %+v", choices)
	}
	if connected.PageName != "Clínica Sol" || connected.InstagramUsername != "clinicasol" || !connected.HasInstagram {
		t.Errorf("connected = %+v", connected)
	}

	got := rows(t, app, "clinic-1")
	messenger, instagram := got[social.Messenger], got[social.Instagram]
	if messenger == nil || instagram == nil {
		t.Fatalf("rows = %v, want one per channel", got)
	}
	if messenger.GetString("external_id") != "P1" || messenger.GetString("access_token") != "PT1" {
		t.Errorf("messenger row = %v", messenger.FieldsData())
	}
	// Instagram is addressed by its own id and sent through the Page.
	if instagram.GetString("external_id") != "IG1" || instagram.GetString("page_id") != "P1" ||
		instagram.GetString("username") != "clinicasol" {
		t.Errorf("instagram row = %v", instagram.FieldsData())
	}

	if !g.saw("POST /v25.0/P1/subscribed_apps") {
		t.Error("the Page was not subscribed to webhooks")
	}
}

func TestTheTokenIsNeverInTheSerializedRow(t *testing.T) {
	app := newApp(t)
	g := newGraph(t, onePageWithInstagram)

	if _, _, err := social.NewConnector(app, g.client()).Start(context.Background(), "clinic-1", "CODE"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	encoded, _ := json.Marshal(rows(t, app, "clinic-1")[social.Messenger])
	if strings.Contains(string(encoded), "PT1") {
		t.Errorf("the Page token is in the serialized row: %s", encoded)
	}
}

func TestSeveralPagesAreOfferedAndTheChosenOneConnected(t *testing.T) {
	app := newApp(t)
	g := newGraph(t, twoPages)
	connector := social.NewConnector(app, g.client())

	connected, choices, err := connector.Start(context.Background(), "clinic-1", "CODE")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if connected != nil || len(choices) != 2 {
		t.Fatalf("connected = %+v, choices = %+v", connected, choices)
	}
	if choices[0].InstagramUsername != "clinicasol" {
		t.Errorf("choices[0] = %+v", choices[0])
	}
	if len(rows(t, app, "clinic-1")) != 0 {
		t.Error("something was stored before a Page was chosen")
	}

	connected, err = connector.Choose(context.Background(), "clinic-1", "P2")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if connected.PageName != "Clínica Luna" || connected.HasInstagram {
		t.Errorf("connected = %+v", connected)
	}
	if got := rows(t, app, "clinic-1"); got[social.Messenger] == nil || got[social.Instagram] != nil {
		t.Errorf("rows = %v, want Messenger only", got)
	}

	// The list is spent.
	if _, err := connector.Choose(context.Background(), "clinic-1", "P1"); !errors.Is(err, social.ErrChoiceExpired) {
		t.Errorf("second Choose err = %v, want ErrChoiceExpired", err)
	}
}

func TestAPageNotOfferedCannotBeChosen(t *testing.T) {
	app := newApp(t)
	connector := social.NewConnector(app, newGraph(t, twoPages).client())

	if _, _, err := connector.Start(context.Background(), "clinic-1", "CODE"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := connector.Choose(context.Background(), "clinic-1", "P-OTHER"); !errors.Is(err, social.ErrUnknownPage) {
		t.Errorf("err = %v, want ErrUnknownPage", err)
	}
	// Nor can another clinic choose from this clinic's list.
	if _, err := connector.Choose(context.Background(), "clinic-2", "P1"); !errors.Is(err, social.ErrChoiceExpired) {
		t.Errorf("err = %v, want ErrChoiceExpired", err)
	}
}

func TestALoginThatGrantedNoPageSaysSo(t *testing.T) {
	app := newApp(t)

	_, _, err := social.NewConnector(app, newGraph(t, `{"data":[]}`).client()).Start(context.Background(), "clinic-1", "CODE")
	if !errors.Is(err, social.ErrNoPages) {
		t.Errorf("err = %v, want ErrNoPages", err)
	}
}

func TestAPageAnotherClinicHasIsRefusedAndNotSubscribed(t *testing.T) {
	app := newApp(t)

	if _, _, err := social.NewConnector(app, newGraph(t, onePageWithInstagram).client()).Start(context.Background(), "clinic-1", "CODE"); err != nil {
		t.Fatalf("first Start: %v", err)
	}

	g := newGraph(t, onePageWithInstagram)
	_, _, err := social.NewConnector(app, g.client()).Start(context.Background(), "clinic-2", "CODE")
	if !errors.Is(err, social.ErrTaken) {
		t.Fatalf("err = %v, want ErrTaken", err)
	}
	if g.saw("POST /v25.0/P1/subscribed_apps") {
		t.Error("a refused connection still subscribed the Page")
	}
	if len(rows(t, app, "clinic-2")) != 0 {
		t.Error("the refused clinic got rows")
	}
}

func TestAFailedSubscriptionStoresNothing(t *testing.T) {
	app := newApp(t)
	g := newGraph(t, onePageWithInstagram)
	g.subscribe = `{"success":false}`

	if _, _, err := social.NewConnector(app, g.client()).Start(context.Background(), "clinic-1", "CODE"); err == nil {
		t.Fatal("a refused subscription was reported as connected")
	}
	if len(rows(t, app, "clinic-1")) != 0 {
		t.Error("rows were stored for a Page Meta will not deliver")
	}
}

func TestConnectingAnotherPageReplacesTheFirstAndUnsubscribesIt(t *testing.T) {
	app := newApp(t)

	if _, _, err := social.NewConnector(app, newGraph(t, onePageWithInstagram).client()).Start(context.Background(), "clinic-1", "CODE"); err != nil {
		t.Fatalf("first Start: %v", err)
	}

	g := newGraph(t, `{"data":[{"id":"P2","name":"Clínica Luna","access_token":"PT2"}]}`)
	if _, _, err := social.NewConnector(app, g.client()).Start(context.Background(), "clinic-1", "CODE"); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	got := rows(t, app, "clinic-1")
	if got[social.Messenger].GetString("external_id") != "P2" {
		t.Errorf("messenger row = %v", got[social.Messenger].FieldsData())
	}
	if got[social.Instagram] != nil {
		t.Error("the first Page's Instagram is still connected")
	}
	if !g.saw("DELETE /v25.0/P1/subscribed_apps") {
		t.Error("the replaced Page was not unsubscribed")
	}
}

func TestDisconnectingMessengerKeepsThePageSubscribedForInstagram(t *testing.T) {
	app := newApp(t)
	g := newGraph(t, onePageWithInstagram)
	connector := social.NewConnector(app, g.client())

	if _, _, err := connector.Start(context.Background(), "clinic-1", "CODE"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := connector.Disconnect(context.Background(), "clinic-1", social.Messenger); err != nil {
		t.Fatalf("Disconnect messenger: %v", err)
	}
	if g.saw("DELETE /v25.0/P1/subscribed_apps") {
		t.Error("the Page was unsubscribed while Instagram still needs it")
	}

	if err := connector.Disconnect(context.Background(), "clinic-1", social.Instagram); err != nil {
		t.Fatalf("Disconnect instagram: %v", err)
	}
	if !g.saw("DELETE /v25.0/P1/subscribed_apps") {
		t.Error("the Page stayed subscribed with nothing connected")
	}
	if len(rows(t, app, "clinic-1")) != 0 {
		t.Error("rows remain after disconnecting both")
	}
}
