package whatsapp_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/whatsapp"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// seededToken is the access token the stubbed account holds. No response may
// contain it: the browser is never allowed to send on Meta's behalf itself.
const seededToken = "EAAG-account-token-must-not-leak"

// templateGraph stands in for Meta. It answers the two template endpoints and
// records every request, so a test can assert which WABA was addressed and
// with whose token.
type templateGraph struct {
	server *httptest.Server

	requests []graphRequest

	list   string
	create string
	send   string

	// status maps a path suffix to the status its reply carries, so one call
	// can be broken while the others keep working.
	status map[string]int
}

type graphRequest struct {
	method string
	path   string
	query  string
	bearer string
	body   string
}

func newTemplateGraph(t *testing.T) *templateGraph {
	t.Helper()

	graph := &templateGraph{
		list: `{"data":[
			{"name":"appointment_reminder","status":"APPROVED","category":"UTILITY","language":"es",
			 "components":[{"type":"BODY","text":"Hola {{1}}, te recordamos tu cita {{2}}."}],"id":"1"},
			{"name":"welcome","status":"PENDING","category":"MARKETING","language":"en_US",
			 "components":[{"type":"BODY","text":"Welcome!"}],"id":"2"}
		]}`,
		create: `{"id":"123456789012345","status":"PENDING","category":"UTILITY"}`,
		send:   `{"messaging_product":"whatsapp","messages":[{"id":"wamid.TEST"}]}`,
		status: map[string]int{},
	}

	graph.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read stub request body: %v", err)
		}

		graph.requests = append(graph.requests, graphRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			bearer: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
			body:   string(body),
		})

		var payload string
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			payload = graph.send
		case r.Method == http.MethodPost:
			payload = graph.create
		default:
			payload = graph.list
		}

		status := http.StatusOK
		for suffix, code := range graph.status {
			if strings.HasSuffix(r.URL.Path, suffix) && (code >= 400 || status == http.StatusOK) {
				status = code
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Errorf("write stub response: %v", err)
		}
	}))
	t.Cleanup(graph.server.Close)

	return graph
}

func (g *templateGraph) client() *meta.Client {
	c := meta.New("APPID", "APPSECRET")
	c.BaseURL = g.server.URL

	return c
}

// connectedAccount gives userID a connected Cloud API account, the way a
// finished signup would have left one.
func connectedAccount(t *testing.T, app core.App, userID, wabaID, phoneNumberID string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(migrations.WhatsAppAccountsCollection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(whatsapp_accounts): %v", err)
	}

	account := core.NewRecord(collection)
	account.Set("user_id", userID)
	account.Set("session_id", "business_"+userID)
	account.Set("account_type", whatsapp.BusinessAPIAccount)
	account.Set("waba_id", wabaID)
	account.Set("phone_number_id", phoneNumberID)
	account.Set("access_token", seededToken)
	account.Set("status", "connected")
	account.Set("is_active", true)
	if err := app.Save(account); err != nil {
		t.Fatalf("Save whatsapp account: %v", err)
	}

	return account
}

func send(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

// decode reads a JSON response body, failing the test if it is not JSON.
func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body, err)
	}

	return body
}

// The list is the account's own: the WABA addressed comes from the signed-in
// user's stored account, not from a constant in the handler.
func TestListTemplatesReadsTheSignedInUsersOwnWABA(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")
	graph := newTemplateGraph(t)
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodGet, "/api/whatsapp/templates", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	body := decode(t, rec)
	templates, ok := body["templates"].([]any)
	if !ok {
		t.Fatalf("body = %s, want a templates array", rec.Body)
	}
	if len(templates) != 2 {
		t.Fatalf("got %d templates, want 2", len(templates))
	}

	first, _ := templates[0].(map[string]any)
	if first["name"] != "appointment_reminder" || first["status"] != "APPROVED" {
		t.Errorf("first template = %v, want the approved reminder passed through verbatim", first)
	}

	if len(graph.requests) != 1 {
		t.Fatalf("made %d Graph calls, want 1: %v", len(graph.requests), graph.requests)
	}
	got := graph.requests[0]
	if got.path != "/v21.0/999000111222333/message_templates" {
		t.Errorf("path = %q, want the account's own WABA at v21.0", got.path)
	}
	if got.bearer != seededToken {
		t.Errorf("bearer = %q, want the account's access token", got.bearer)
	}
	if strings.Contains(rec.Body.String(), seededToken) {
		t.Errorf("the response leaks the account access token: %s", rec.Body)
	}
}

// Every template route spends an account token, so an anonymous caller must
// not reach Meta at all.
func TestTemplateRoutesRequireASignedInUser(t *testing.T) {
	app := newApp(t)
	graph := newTemplateGraph(t)
	mux := newMux(t, app, graph.client(), nil)

	for _, call := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/whatsapp/templates", ""},
		{http.MethodPost, "/api/whatsapp/templates/create", `{"name":"n","category":"UTILITY","language":"es","body":"b"}`},
		{http.MethodPost, "/api/whatsapp/templates/send", `{"template_name":"n","language_code":"es","to":"5216641234567"}`},
	} {
		rec := send(t, mux, call.method, call.path, call.body)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", call.method, call.path, rec.Code)
		}
	}

	if len(graph.requests) != 0 {
		t.Errorf("an anonymous request reached Meta: %v", graph.requests)
	}
}

// A person with no connected number gets told that, rather than a 500 or an
// empty list that looks like "you have no templates".
func TestTemplateRoutesSayWhenNoNumberIsConnected(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "nonumber@example.test")
	graph := newTemplateGraph(t)
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodGet, "/api/whatsapp/templates", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body)
	}
	if message, _ := decode(t, rec)["message"].(string); !strings.Contains(message, "No connected WhatsApp account") {
		t.Errorf("message = %q, want it to say no account is connected", message)
	}
	if len(graph.requests) != 0 {
		t.Errorf("a request without an account reached Meta: %v", graph.requests)
	}
}

// Another tenant's templates are not this tenant's business: the account is
// resolved from the verified session, so there is nothing in the request to
// point at someone else's WABA.
func TestListTemplatesIgnoresAnotherUsersAccount(t *testing.T) {
	app := newApp(t)
	mine := newUser(t, app, "mine@example.test")
	theirs := newUser(t, app, "theirs@example.test")
	connectedAccount(t, app, mine.Id, "111111111111111", "200000000000001")
	connectedAccount(t, app, theirs.Id, "222222222222222", "200000000000002")
	graph := newTemplateGraph(t)
	mux := newMux(t, app, graph.client(), mine)

	if rec := send(t, mux, http.MethodGet, "/api/whatsapp/templates", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	if got := graph.requests[0].path; !strings.Contains(got, "111111111111111") {
		t.Errorf("path = %q, want the caller's own WABA", got)
	}
}

// Meta's own wording is the only thing that explains the failure, but its raw
// body is not fit to send to a browser.
func TestListTemplatesSurfacesMetasMessageWithoutItsBody(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")
	graph := newTemplateGraph(t)
	graph.status["/message_templates"] = http.StatusBadRequest
	graph.list = `{"error":{"message":"Error validating access token: Session has expired.",` +
		`"type":"OAuthException","code":190,"fbtrace_id":"AbCdEfG"}}`
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodGet, "/api/whatsapp/templates", "")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rec.Code, rec.Body)
	}

	message, _ := decode(t, rec)["message"].(string)
	if !strings.Contains(message, "Session has expired") {
		t.Errorf("message = %q, want Meta's own explanation", message)
	}
	// fbtrace_id, the type and the rest of the envelope are log material, not
	// something a browser is shown.
	if strings.Contains(rec.Body.String(), "fbtrace_id") || strings.Contains(rec.Body.String(), "OAuthException") {
		t.Errorf("the response echoes Meta's raw error body: %s", rec.Body)
	}
}

// A submitted template reaches Meta as the one-component BODY draft the
// create endpoint accepts, against the caller's own WABA.
func TestCreateTemplateSubmitsTheDraftToTheCallersWABA(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")
	graph := newTemplateGraph(t)
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodPost, "/api/whatsapp/templates/create",
		`{"name":"appointment_reminder","category":"UTILITY","language":"es","body":"Hola {{1}}, tu cita es {{2}}."}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	body := decode(t, rec)
	if body["success"] != true {
		t.Errorf("body = %s, want success true", rec.Body)
	}
	template, ok := body["template"].(map[string]any)
	if !ok || template["status"] != "PENDING" {
		t.Errorf("template = %v, want Meta's response passed through", body["template"])
	}

	if len(graph.requests) != 1 {
		t.Fatalf("made %d Graph calls, want 1: %v", len(graph.requests), graph.requests)
	}
	got := graph.requests[0]
	if got.method != http.MethodPost || got.path != "/v21.0/999000111222333/message_templates" {
		t.Errorf("%s %s, want POST to the caller's WABA at v21.0", got.method, got.path)
	}
	if got.bearer != seededToken {
		t.Errorf("bearer = %q, want the account's access token", got.bearer)
	}
	// Uppercase BODY on create; lowercase body on send. Meta rejects the
	// wrong one and nothing else in the request says which call this is.
	if !strings.Contains(got.body, `"type":"BODY"`) {
		t.Errorf("request body = %s, want an uppercase BODY component", got.body)
	}
	if !strings.Contains(got.body, `{{1}}`) {
		t.Errorf("request body = %s, want the placeholders kept verbatim", got.body)
	}
	if strings.Contains(rec.Body.String(), seededToken) {
		t.Errorf("the response leaks the account access token: %s", rec.Body)
	}
}

// The form validates too, but the endpoint is reachable without it, and a
// half-filled draft is a bad request rather than a Meta round trip.
func TestCreateTemplateRejectsAnIncompleteDraft(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")

	for name, payload := range map[string]string{
		"no name":     `{"name":"","category":"UTILITY","language":"es","body":"b"}`,
		"no category": `{"name":"n","category":"","language":"es","body":"b"}`,
		"no language": `{"name":"n","category":"UTILITY","language":"","body":"b"}`,
		"no body":     `{"name":"n","category":"UTILITY","language":"es","body":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			graph := newTemplateGraph(t)
			mux := newMux(t, app, graph.client(), user)

			rec := send(t, mux, http.MethodPost, "/api/whatsapp/templates/create", payload)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
			}
			if len(graph.requests) != 0 {
				t.Errorf("an incomplete draft reached Meta: %v", graph.requests)
			}
		})
	}
}

// "Template name already exists" is the whole answer to a failed create, so
// it has to survive the trip back to the page.
func TestCreateTemplateSurfacesMetasRejection(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")
	graph := newTemplateGraph(t)
	graph.status["/message_templates"] = http.StatusBadRequest
	graph.create = `{"error":{"message":"Template name (appointment_reminder) already exists in es",` +
		`"type":"OAuthException","code":100,"error_user_title":"Template Name Already Exists","fbtrace_id":"AbCdEfG"}}`
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodPost, "/api/whatsapp/templates/create",
		`{"name":"appointment_reminder","category":"UTILITY","language":"es","body":"hi"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rec.Code, rec.Body)
	}

	message, _ := decode(t, rec)["message"].(string)
	if !strings.Contains(message, "already exists") {
		t.Errorf("message = %q, want Meta's own rejection", message)
	}
	if strings.Contains(rec.Body.String(), "fbtrace_id") {
		t.Errorf("the response echoes Meta's raw error body: %s", rec.Body)
	}
}

// A test send goes out from the account's own number and comes back with the
// WhatsApp message id, which is what proves it left.
func TestSendTemplateSendsFromTheCallersOwnNumber(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")
	graph := newTemplateGraph(t)
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodPost, "/api/whatsapp/templates/send",
		`{"template_name":"appointment_reminder","language_code":"es","to":"5216641234567"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	body := decode(t, rec)
	if body["success"] != true || body["message_id"] != "wamid.TEST" {
		t.Errorf("body = %s, want success and the WhatsApp message id", rec.Body)
	}

	if len(graph.requests) != 1 {
		t.Fatalf("made %d Graph calls, want 1: %v", len(graph.requests), graph.requests)
	}
	got := graph.requests[0]
	if got.method != http.MethodPost || got.path != "/v21.0/200000000000002/messages" {
		t.Errorf("%s %s, want POST to the account's phone number id at v21.0", got.method, got.path)
	}
	if got.bearer != seededToken {
		t.Errorf("bearer = %q, want the account's access token", got.bearer)
	}
	if !strings.Contains(got.body, `"type":"template"`) || !strings.Contains(got.body, `"to":"5216641234567"`) {
		t.Errorf("request body = %s, want a template send to the given number", got.body)
	}
	// A template with no placeholders must omit components entirely; Meta
	// rejects an empty array.
	if strings.Contains(got.body, `"components"`) {
		t.Errorf("request body = %s, want no components for a template without parameters", got.body)
	}
	if strings.Contains(rec.Body.String(), seededToken) {
		t.Errorf("the response leaks the account access token: %s", rec.Body)
	}
}

// Most templates worth sending have placeholders — the reminder this app
// exists to send has two — so a send that cannot fill them is not a send.
func TestSendTemplateFillsBodyParameters(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")
	graph := newTemplateGraph(t)
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodPost, "/api/whatsapp/templates/send",
		`{"template_name":"appointment_reminder","language_code":"es","to":"5216641234567",`+
			`"body_parameters":["Ana","mañana a las 10"]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	got := graph.requests[0].body
	// Lowercase "body" on send, uppercase "BODY" on create.
	if !strings.Contains(got, `"type":"body"`) {
		t.Errorf("request body = %s, want a lowercase body component", got)
	}
	if !strings.Contains(got, `{"type":"text","text":"Ana"}`) ||
		!strings.Contains(got, `{"type":"text","text":"mañana a las 10"}`) {
		t.Errorf("request body = %s, want both parameters in order", got)
	}
}

func TestSendTemplateRejectsAnIncompleteSend(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")

	for name, payload := range map[string]string{
		"no template": `{"template_name":"","language_code":"es","to":"5216641234567"}`,
		"no language": `{"template_name":"n","language_code":"","to":"5216641234567"}`,
		"no number":   `{"template_name":"n","language_code":"es","to":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			graph := newTemplateGraph(t)
			mux := newMux(t, app, graph.client(), user)

			rec := send(t, mux, http.MethodPost, "/api/whatsapp/templates/send", payload)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
			}
			if len(graph.requests) != 0 {
				t.Errorf("an incomplete send reached Meta: %v", graph.requests)
			}
		})
	}
}

// A number outside the test allow list is the failure a person actually hits,
// and Meta's wording is what tells them to add it.
func TestSendTemplateSurfacesMetasRefusal(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	connectedAccount(t, app, user.Id, "999000111222333", "200000000000002")
	graph := newTemplateGraph(t)
	graph.status["/messages"] = http.StatusBadRequest
	graph.send = `{"error":{"message":"Recipient phone number not in allowed list",` +
		`"type":"OAuthException","code":131030,"error_data":{"details":"the token is EAAG-secret"},"fbtrace_id":"AbCdEfG"}}`
	mux := newMux(t, app, graph.client(), user)

	rec := send(t, mux, http.MethodPost, "/api/whatsapp/templates/send",
		`{"template_name":"appointment_reminder","language_code":"es","to":"15550000000"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rec.Code, rec.Body)
	}

	message, _ := decode(t, rec)["message"].(string)
	if !strings.Contains(message, "not in allowed list") {
		t.Errorf("message = %q, want Meta's own refusal", message)
	}
	if strings.Contains(rec.Body.String(), "fbtrace_id") || strings.Contains(rec.Body.String(), "error_data") {
		t.Errorf("the response echoes Meta's raw error body: %s", rec.Body)
	}
}
