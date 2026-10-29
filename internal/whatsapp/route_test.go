package whatsapp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/whatsapp"
)

// newMux builds the application's routes on a bare router, with auth supplied
// by the caller rather than by a real token, so the handler can be exercised
// over HTTP without a listener or a login.
func newMux(t *testing.T, app core.App, client *meta.Client, auth *core.Record) http.Handler {
	t.Helper()

	r := router.NewRouter(func(w http.ResponseWriter, req *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		e := new(core.RequestEvent)
		e.App = app
		e.Auth = auth
		e.Response = w
		e.Request = req
		return e, nil
	})

	whatsapp.Register(r, client)

	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	return mux
}

func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/whatsapp/exchange-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

// The endpoint spends the app secret and writes an account for whoever is
// signed in, so an unauthenticated caller must not reach it at all.
func TestExchangeTokenRequiresASignedInUser(t *testing.T) {
	app := newApp(t)
	graph := newFakeGraph(t)
	mux := newMux(t, app, graph.client(), nil)

	rec := post(t, mux, `{"code":"c","waba_id":"w","phone_number_id":"555000111"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if len(graph.paths) != 0 {
		t.Errorf("an anonymous request reached Meta: %v", graph.paths)
	}
}

func TestExchangeTokenConnectsTheSignedInUsersAccount(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	mux := newMux(t, app, newFakeGraph(t).client(), user)

	rec := post(t, mux, `{"code":"the-code","waba_id":"506463685879575"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body, err)
	}
	if body["success"] != true {
		t.Errorf("body = %s, want success true", rec.Body)
	}
	// The response is what the browser sees, so the token must not be in it
	// even though the handler just wrote one.
	if strings.Contains(rec.Body.String(), "EAAG") {
		t.Errorf("the access token is in the response body: %s", rec.Body)
	}

	account, err := whatsapp.AccountFor(app, user.Id)
	if err != nil {
		t.Fatalf("no account was stored for the signed-in user: %v", err)
	}
	if got := account.GetString("phone_number_id"); got != "555000111" {
		t.Errorf("phone_number_id = %q", got)
	}
}

// A signup this app cannot complete is the caller's problem, not a server
// fault, and the UI shows the message, so it has to say something useful.
func TestExchangeTokenAnswers400ForASignupItCannotComplete(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")

	t.Run("missing fields", func(t *testing.T) {
		mux := newMux(t, app, newFakeGraph(t).client(), user)

		rec := post(t, mux, `{"waba_id":"w"}`)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("ambiguous phone number", func(t *testing.T) {
		graph := newFakeGraph(t)
		graph.phoneNumbers = `{"data":[{"id":"1"},{"id":"2"}]}`
		mux := newMux(t, app, graph.client(), user)

		rec := post(t, mux, `{"code":"c","waba_id":"w"}`)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

// Meta's own wording ("this authorization code has been used") is the only
// thing that tells a person what went wrong, but it must not arrive with a 200
// or be mistaken for success.
func TestExchangeTokenReportsAMetaFailureAsAServerError(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	graph := newFakeGraph(t)
	graph.status["/oauth/access_token"] = http.StatusBadRequest
	graph.token = `{"error":{"message":"This authorization code has been used.","code":100}}`
	mux := newMux(t, app, graph.client(), user)

	rec := post(t, mux, `{"code":"used","waba_id":"w","phone_number_id":"555000111"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	// The app secret travels in the failing request's query string, so Meta's
	// raw response is not safe to echo to the browser.
	if strings.Contains(rec.Body.String(), "APPSECRET") {
		t.Errorf("the response leaks the app secret: %s", rec.Body)
	}
}
