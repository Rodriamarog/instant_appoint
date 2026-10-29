package whatsapp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/whatsapp"
	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// newApp returns a bootstrapped app with the schema applied, backed by a
// throwaway data directory.
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

func newUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(users): %v", err)
	}

	user := core.NewRecord(collection)
	user.SetEmail(email)
	user.SetPassword("password1234")
	user.SetVerified(true)
	if err := app.Save(user); err != nil {
		t.Fatalf("Save user: %v", err)
	}

	return user
}

// fakeGraph answers the four Graph calls the signup makes. Each reply is a
// field so a test can break exactly one of them.
type fakeGraph struct {
	server *httptest.Server

	paths []string

	token        string
	phoneNumbers string
	subscribe    string
	verifiedName string

	status map[string]int
}

func newFakeGraph(t *testing.T) *fakeGraph {
	t.Helper()

	graph := &fakeGraph{
		token:        `{"access_token":"EAAG-the-secret-token","token_type":"bearer"}`,
		phoneNumbers: `{"data":[{"id":"555000111"}]}`,
		subscribe:    `{"success":true}`,
		verifiedName: `{"verified_name":"Dev Clinic","display_phone_number":"+52 1 664 123 4567","id":"555000111"}`,
		status:       map[string]int{},
	}

	graph.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graph.paths = append(graph.paths, r.URL.Path)

		var payload string
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth/access_token"):
			payload = graph.token
		case strings.HasSuffix(r.URL.Path, "/phone_numbers"):
			payload = graph.phoneNumbers
		case strings.HasSuffix(r.URL.Path, "/subscribed_apps"):
			payload = graph.subscribe
		default:
			payload = graph.verifiedName
		}

		status := http.StatusOK
		for suffix, code := range graph.status {
			if strings.HasSuffix(r.URL.Path, suffix) {
				status = code
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Errorf("write fake graph response: %v", err)
		}
	}))
	t.Cleanup(graph.server.Close)

	return graph
}

func (g *fakeGraph) client() *meta.Client {
	c := meta.New("APPID", "APPSECRET")
	c.BaseURL = g.server.URL
	return c
}

func (g *fakeGraph) called(suffix string) bool {
	for _, path := range g.paths {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	return false
}

// The whole point of the ticket: a signup turns into a stored, connected
// account whose token the server can read and the browser cannot.
func TestConnectStoresTheAccountWithTheTokenAndTheVerifiedName(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	graph := newFakeGraph(t)

	account, err := whatsapp.Connect(context.Background(), app, graph.client(), user.Id, whatsapp.Signup{
		Code:          "the-signup-code",
		WABAID:        "506463685879575",
		PhoneNumberID: "555000111",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	for field, want := range map[string]string{
		"user_id":         user.Id,
		"account_type":    "business_api",
		"waba_id":         "506463685879575",
		"phone_number_id": "555000111",
		"status":          "connected",
		"session_id":      "business_" + user.Id,
		// Fetched from Meta with the v21.0 query-param call. The legacy
		// schema never declared this field, which took the whole exchange
		// down rather than just leaving a label blank.
		"verified_name": "Dev Clinic",
		// The token is readable in Go even though it is hidden from the API.
		"access_token": "EAAG-the-secret-token",
	} {
		if got := account.GetString(field); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if !account.GetBool("is_active") {
		t.Error("is_active is false on a freshly connected account")
	}

	// Without the subscription Meta delivers no webhooks and the account is
	// connected but deaf, which looks like a working connection from the UI.
	if !graph.called("/subscribed_apps") {
		t.Error("the WABA webhook subscription was never requested")
	}
}

// Hidden is what keeps the token out of the browser. Asserting it on the
// serialized record is the check that survives someone editing the migration.
func TestConnectLeavesTheTokenOutOfTheSerializedRecord(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")

	account, err := whatsapp.Connect(context.Background(), app, newFakeGraph(t).client(), user.Id, whatsapp.Signup{
		Code: "code", WABAID: "waba", PhoneNumberID: "555000111",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	encoded, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, secret := range []string{"access_token", "EAAG-the-secret-token", "qr_code", "session_id"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("%q appears in the serialized record: %s", secret, encoded)
		}
	}
}

// A business that stays on the WhatsApp Business app finishes signup without a
// phone_number_id. That is the common case, and the WABA's only number is the
// answer.
func TestConnectFallsBackToTheWABAsOnlyNumber(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	graph := newFakeGraph(t)

	account, err := whatsapp.Connect(context.Background(), app, graph.client(), user.Id, whatsapp.Signup{
		Code: "code", WABAID: "506463685879575",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if got := account.GetString("phone_number_id"); got != "555000111" {
		t.Errorf("phone_number_id = %q, want the WABA's only number", got)
	}
	if !graph.called("/phone_numbers") {
		t.Error("the WABA's numbers were never looked up")
	}
}

// Storing an account against a number nobody can identify would send every
// later message from the wrong place, so the signup fails instead.
func TestConnectRefusesAnAmbiguousNumberWithoutWritingAnything(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	graph := newFakeGraph(t)
	graph.phoneNumbers = `{"data":[{"id":"1"},{"id":"2"}]}`

	_, err := whatsapp.Connect(context.Background(), app, graph.client(), user.Id, whatsapp.Signup{
		Code: "code", WABAID: "waba",
	})
	if !errors.Is(err, meta.ErrPhoneNumberUnresolved) {
		t.Fatalf("err = %v, want ErrPhoneNumberUnresolved", err)
	}

	records, err := app.FindAllRecords("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("wrote %d accounts for a signup that failed", len(records))
	}
}

// Reconnecting is routine — a token expires, a number is re-onboarded — and it
// must land on the same row rather than growing a second one.
func TestConnectUpdatesTheUsersExistingBusinessAccount(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	graph := newFakeGraph(t)

	first, err := whatsapp.Connect(context.Background(), app, graph.client(), user.Id, whatsapp.Signup{
		Code: "code", WABAID: "waba", PhoneNumberID: "555000111",
	})
	if err != nil {
		t.Fatalf("first Connect: %v", err)
	}

	graph.token = `{"access_token":"EAAG-a-fresh-token"}`
	second, err := whatsapp.Connect(context.Background(), app, graph.client(), user.Id, whatsapp.Signup{
		Code: "code2", WABAID: "waba", PhoneNumberID: "555000222",
	})
	if err != nil {
		t.Fatalf("second Connect: %v", err)
	}

	if first.Id != second.Id {
		t.Errorf("a reconnection created a second row: %q then %q", first.Id, second.Id)
	}
	if got := second.GetString("access_token"); got != "EAAG-a-fresh-token" {
		t.Errorf("access_token = %q, want the token from the second exchange", got)
	}

	records, err := app.FindAllRecords("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("there are %d accounts after reconnecting, want 1", len(records))
	}
}

// The route this was ported from matched on user_id alone, so a signup
// overwrote a legacy QR account in place and the user lost it.
func TestConnectLeavesALegacyQRAccountAlone(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")

	collection, err := app.FindCollectionByNameOrId("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}
	legacy := core.NewRecord(collection)
	legacy.Set("user_id", user.Id)
	legacy.Set("account_type", "web")
	legacy.Set("session_id", user.Id)
	legacy.Set("status", "connected")
	legacy.Set("is_active", true)
	legacy.Set("phone_number", "5216641234567")
	if err := app.Save(legacy); err != nil {
		t.Fatalf("Save legacy account: %v", err)
	}

	account, err := whatsapp.Connect(context.Background(), app, newFakeGraph(t).client(), user.Id, whatsapp.Signup{
		Code: "code", WABAID: "waba", PhoneNumberID: "555000111",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if account.Id == legacy.Id {
		t.Fatal("the signup overwrote the legacy QR account in place")
	}

	reloaded, err := app.FindRecordById("whatsapp_accounts", legacy.Id)
	if err != nil {
		t.Fatalf("the legacy account is gone: %v", err)
	}
	if got := reloaded.GetString("account_type"); got != "web" {
		t.Errorf("legacy account_type = %q, want web", got)
	}
}

// Two accounts belonging to two people must not collide, and a signup must
// never land on somebody else's row.
func TestConnectKeepsAccountsSeparatePerPerson(t *testing.T) {
	app := newApp(t)
	graph := newFakeGraph(t)
	first := newUser(t, app, "one@example.test")
	second := newUser(t, app, "two@example.test")

	a, err := whatsapp.Connect(context.Background(), app, graph.client(), first.Id, whatsapp.Signup{
		Code: "code", WABAID: "waba-one", PhoneNumberID: "111",
	})
	if err != nil {
		t.Fatalf("Connect(first): %v", err)
	}
	b, err := whatsapp.Connect(context.Background(), app, graph.client(), second.Id, whatsapp.Signup{
		Code: "code", WABAID: "waba-two", PhoneNumberID: "222",
	})
	if err != nil {
		t.Fatalf("Connect(second): %v", err)
	}

	if a.Id == b.Id {
		t.Fatal("the second person's signup landed on the first person's row")
	}
	if got := a.GetString("waba_id"); got != "waba-one" {
		t.Errorf("first waba_id = %q", got)
	}
	if got := b.GetString("waba_id"); got != "waba-two" {
		t.Errorf("second waba_id = %q", got)
	}
}

// Both of these are nice-to-haves next to a working connection: a subscription
// can be retried and a missing display name is a cosmetic gap, so neither is
// worth throwing away a successfully exchanged token for.
func TestConnectSurvivesAFailedSubscriptionOrVerifiedNameLookup(t *testing.T) {
	for name, fail := range map[string]func(*fakeGraph){
		"refused subscription": func(g *fakeGraph) { g.subscribe = `{"success":false}` },
		"verified name 400": func(g *fakeGraph) {
			g.status["/555000111"] = http.StatusBadRequest
			g.verifiedName = `{"error":{"message":"Unsupported get request.","code":100}}`
		},
	} {
		t.Run(name, func(t *testing.T) {
			app := newApp(t)
			user := newUser(t, app, "owner@example.test")
			graph := newFakeGraph(t)
			fail(graph)

			account, err := whatsapp.Connect(context.Background(), app, graph.client(), user.Id, whatsapp.Signup{
				Code: "code", WABAID: "waba", PhoneNumberID: "555000111",
			})
			if err != nil {
				t.Fatalf("Connect: %v", err)
			}
			if got := account.GetString("status"); got != "connected" {
				t.Errorf("status = %q, want connected", got)
			}
		})
	}
}

// phone_number_id is optional on the way in — the coexistence signup omits it —
// but without a code and a WABA there is nothing to exchange.
func TestConnectRequiresACodeAndAWABA(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")

	for name, signup := range map[string]whatsapp.Signup{
		"no code": {WABAID: "waba"},
		"no waba": {Code: "code"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := whatsapp.Connect(context.Background(), app, newFakeGraph(t).client(), user.Id, signup)
			if !errors.Is(err, whatsapp.ErrIncompleteSignup) {
				t.Errorf("err = %v, want ErrIncompleteSignup", err)
			}
		})
	}
}

// A reused or expired authorization code is the most common real failure, and
// it must not leave a half-written account behind.
func TestConnectReportsARejectedCode(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	graph := newFakeGraph(t)
	graph.status["/oauth/access_token"] = http.StatusBadRequest
	graph.token = `{"error":{"message":"This authorization code has been used.","code":100,"error_subcode":36009}}`

	_, err := whatsapp.Connect(context.Background(), app, graph.client(), user.Id, whatsapp.Signup{
		Code: "used", WABAID: "waba", PhoneNumberID: "555000111",
	})

	var apiErr *meta.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T, want *meta.APIError", err)
	}

	records, err := app.FindAllRecords("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("wrote %d accounts after a rejected code", len(records))
	}
}
