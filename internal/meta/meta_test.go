package meta_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
)

// graphStub stands in for graph.facebook.com. It records the one request it
// was given so a test can assert the literal wire contract — path (which
// carries the API version), query and headers are all part of what Meta
// accepts, and getting any of them wrong is invisible until someone runs the
// real signup.
type graphStub struct {
	server *httptest.Server

	calls []*recorded
	reply func(r *http.Request) (int, string)
}

type recorded struct {
	method string
	path   string
	query  map[string][]string
	header http.Header
	body   string
}

func newGraphStub(t *testing.T, reply func(r *http.Request) (int, string)) *graphStub {
	t.Helper()

	stub := &graphStub{reply: reply}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			if _, err := r.Body.Read(body); err != nil && err.Error() != "EOF" {
				t.Errorf("read stub request body: %v", err)
			}
		}

		stub.calls = append(stub.calls, &recorded{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			header: r.Header.Clone(),
			body:   string(body),
		})

		status, payload := stub.reply(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Errorf("write stub response: %v", err)
		}
	}))
	t.Cleanup(stub.server.Close)

	return stub
}

func (s *graphStub) client() *meta.Client {
	c := meta.New("APPID", "APPSECRET")
	c.BaseURL = s.server.URL
	return c
}

func (s *graphStub) only(t *testing.T) *recorded {
	t.Helper()

	if len(s.calls) != 1 {
		t.Fatalf("stub saw %d calls, want exactly 1", len(s.calls))
	}

	return s.calls[0]
}

func ok(payload string) func(*http.Request) (int, string) {
	return func(*http.Request) (int, string) { return http.StatusOK, payload }
}

func TestExchangeCodeSendsTheAppCredentialsAndReturnsTheToken(t *testing.T) {
	stub := newGraphStub(t, ok(`{"access_token":"EAAG-long-lived","token_type":"bearer"}`))

	token, err := stub.client().ExchangeCode(context.Background(), "the-signup-code")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if token != "EAAG-long-lived" {
		t.Errorf("token = %q, want %q", token, "EAAG-long-lived")
	}

	call := stub.only(t)
	// The exchange is a GET on v25.0 — not v21.0, and not a POST — and the
	// app secret travels as a query parameter.
	if call.method != http.MethodGet {
		t.Errorf("method = %s, want GET", call.method)
	}
	if call.path != "/v25.0/oauth/access_token" {
		t.Errorf("path = %q, want /v25.0/oauth/access_token", call.path)
	}
	for key, want := range map[string]string{
		"client_id":     "APPID",
		"client_secret": "APPSECRET",
		"code":          "the-signup-code",
	} {
		if got := call.query[key]; len(got) != 1 || got[0] != want {
			t.Errorf("query %s = %v, want [%q]", key, got, want)
		}
	}
}

func TestExchangeCodeReportsMetasError(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusBadRequest, `{"error":{"message":"This authorization code has been used.","type":"OAuthException","code":100}}`
	})

	_, err := stub.client().ExchangeCode(context.Background(), "used-code")
	if err == nil {
		t.Fatal("ExchangeCode succeeded on an error response")
	}

	var apiErr *meta.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T, want *meta.APIError", err)
	}
	// The message is what a human sees when a signup fails, so it has to
	// survive the trip rather than being flattened to "request failed".
	if apiErr.Message != "This authorization code has been used." {
		t.Errorf("Message = %q", apiErr.Message)
	}
}

// Meta answers 200 with an error body often enough that treating the status
// code as the only signal loses real failures.
func TestExchangeCodeTreatsAnErrorBodyOnA200AsAFailure(t *testing.T) {
	stub := newGraphStub(t, ok(`{"error":{"message":"Invalid OAuth access token.","code":190}}`))

	if _, err := stub.client().ExchangeCode(context.Background(), "code"); err == nil {
		t.Fatal("ExchangeCode succeeded on a 200 carrying an error body")
	}
}

// Meta omits phone_number_id when a business keeps using the WhatsApp Business
// app (the coexistence flow), which is the common case rather than an edge
// case, so the WABA's only number is the normal path through this function.
func TestResolvePhoneNumberIDFallsBackToTheWABAsOnlyNumber(t *testing.T) {
	stub := newGraphStub(t, ok(`{"data":[{"id":"555000111"}],"paging":{"cursors":{"before":"a","after":"b"}}}`))

	id, err := stub.client().ResolvePhoneNumberID(context.Background(), "506463685879575", "", "the-token")
	if err != nil {
		t.Fatalf("ResolvePhoneNumberID: %v", err)
	}
	if id != "555000111" {
		t.Errorf("id = %q, want 555000111", id)
	}

	call := stub.only(t)
	if call.method != http.MethodGet {
		t.Errorf("method = %s, want GET", call.method)
	}
	// v25.0 here, and the token is a Bearer header rather than a query param.
	if call.path != "/v25.0/506463685879575/phone_numbers" {
		t.Errorf("path = %q, want /v25.0/506463685879575/phone_numbers", call.path)
	}
	if got := call.query["fields"]; len(got) != 1 || got[0] != "id" {
		t.Errorf("fields = %v, want [id]", got)
	}
	if got := call.header.Get("Authorization"); got != "Bearer the-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer the-token")
	}
}

// A signup that did send the id is the whole answer; calling Meta anyway would
// burn a round trip and could fail for reasons that have nothing to do with
// the signup.
func TestResolvePhoneNumberIDKeepsTheSignupsIDWithoutCallingMeta(t *testing.T) {
	stub := newGraphStub(t, ok(`{"data":[{"id":"999"}]}`))

	id, err := stub.client().ResolvePhoneNumberID(context.Background(), "waba", "555000111", "tok")
	if err != nil {
		t.Fatalf("ResolvePhoneNumberID: %v", err)
	}
	if id != "555000111" {
		t.Errorf("id = %q, want the id from the signup", id)
	}
	if len(stub.calls) != 0 {
		t.Errorf("made %d calls to Meta, want none", len(stub.calls))
	}
}

// Guessing which of several numbers was onboarded would attach the account to
// the wrong number, so an ambiguous or empty answer is a refusal.
func TestResolvePhoneNumberIDRefusesWhenItCannotTell(t *testing.T) {
	for name, payload := range map[string]string{
		"several numbers": `{"data":[{"id":"1"},{"id":"2"}]}`,
		"no numbers":      `{"data":[]}`,
		"no data key":     `{}`,
		"non-string id":   `{"data":[{"id":12345}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			stub := newGraphStub(t, ok(payload))

			_, err := stub.client().ResolvePhoneNumberID(context.Background(), "waba", "", "tok")
			if !errors.Is(err, meta.ErrPhoneNumberUnresolved) {
				t.Errorf("err = %v, want ErrPhoneNumberUnresolved", err)
			}
		})
	}
}

// A rejected token and a WABA with no numbers are different problems, and the
// TypeScript flattened both into "could not determine the phone number". The
// error is kept distinguishable here so a failed signup can say why.
func TestResolvePhoneNumberIDReportsALookupFailureSeparately(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusUnauthorized, `{"error":{"message":"Invalid OAuth access token.","code":190}}`
	})

	_, err := stub.client().ResolvePhoneNumberID(context.Background(), "waba", "", "stale")
	if err == nil {
		t.Fatal("ResolvePhoneNumberID succeeded against a 401")
	}
	if errors.Is(err, meta.ErrPhoneNumberUnresolved) {
		t.Error("a rejected token reads as an unresolvable number; the two need telling apart")
	}

	var apiErr *meta.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T, want *meta.APIError", err)
	}
}

// Without this subscription Meta delivers no webhooks for the account, so
// inbound messages never arrive and the bot is silent.
func TestSubscribeAppPostsWithNothingButTheBearerToken(t *testing.T) {
	stub := newGraphStub(t, ok(`{"success":true}`))

	if err := stub.client().SubscribeApp(context.Background(), "506463685879575", "the-token"); err != nil {
		t.Fatalf("SubscribeApp: %v", err)
	}

	call := stub.only(t)
	if call.method != http.MethodPost {
		t.Errorf("method = %s, want POST", call.method)
	}
	if call.path != "/v25.0/506463685879575/subscribed_apps" {
		t.Errorf("path = %q, want /v25.0/506463685879575/subscribed_apps", call.path)
	}
	if got := call.header.Get("Authorization"); got != "Bearer the-token" {
		t.Errorf("Authorization = %q", got)
	}
	// Meta rejects this call when it carries a JSON body, so the absence of
	// one is part of the contract.
	if call.body != "" {
		t.Errorf("body = %q, want empty", call.body)
	}
	if got := call.header.Get("Content-Type"); got != "" {
		t.Errorf("Content-Type = %q, want unset", got)
	}
}

// Meta answers a refused subscription with 200 and "success": false, so the
// status code alone would report this as a working connection.
func TestSubscribeAppFailsOnASuccessFalseBody(t *testing.T) {
	stub := newGraphStub(t, ok(`{"success":false}`))

	if err := stub.client().SubscribeApp(context.Background(), "waba", "tok"); err == nil {
		t.Fatal("SubscribeApp succeeded on a success:false body")
	}
}

func TestPhoneNumberVerifiedNameUsesTheQueryParamAuthStyle(t *testing.T) {
	stub := newGraphStub(t, ok(`{"verified_name":"Instant Appoint","display_phone_number":"+52 1 664 123 4567","id":"555000111"}`))

	name, err := stub.client().PhoneNumberVerifiedName(context.Background(), "555000111", "the-token")
	if err != nil {
		t.Fatalf("PhoneNumberVerifiedName: %v", err)
	}
	if name != "Instant Appoint" {
		t.Errorf("name = %q, want %q", name, "Instant Appoint")
	}

	call := stub.only(t)
	// This one call is v21.0 while its neighbours in the same handler are
	// v25.0, and it is the only call in the app that passes the token as a
	// query parameter instead of a header.
	if call.path != "/v21.0/555000111" {
		t.Errorf("path = %q, want /v21.0/555000111", call.path)
	}
	if got := call.query["access_token"]; len(got) != 1 || got[0] != "the-token" {
		t.Errorf("access_token query = %v, want [the-token]", got)
	}
	if got := call.header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want unset for this call", got)
	}
	if got := call.query["fields"]; len(got) != 1 || got[0] != "verified_name,display_phone_number" {
		t.Errorf("fields = %v", got)
	}
}

func TestSendTextPostsTheMessageAndReturnsTheWAMID(t *testing.T) {
	stub := newGraphStub(t, ok(`{"messaging_product":"whatsapp","contacts":[{"input":"5215511111111","wa_id":"5215511111111"}],"messages":[{"id":"wamid.HBgNNTIx"}]}`))

	id, err := stub.client().SendText(context.Background(), "555000111", "the-token", "5215511111111", "the reply text")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if id != "wamid.HBgNNTIx" {
		t.Errorf("id = %q", id)
	}

	call := stub.only(t)
	if call.method != http.MethodPost {
		t.Errorf("method = %s, want POST", call.method)
	}
	// Sends are v21.0 while the signup calls are v25.0.
	if call.path != "/v21.0/555000111/messages" {
		t.Errorf("path = %q, want /v21.0/555000111/messages", call.path)
	}
	if got := call.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	want := `{"messaging_product":"whatsapp","to":"5215511111111","type":"text","text":{"body":"the reply text"}}`
	if call.body != want {
		t.Errorf("body  = %s\nwant = %s", call.body, want)
	}
}

func TestSendTemplateOmitsComponentsWhenThereAreNoParameters(t *testing.T) {
	stub := newGraphStub(t, ok(`{"messages":[{"id":"wamid.A"}]}`))

	if _, err := stub.client().SendTemplate(context.Background(), "555000111", "tok", "5215511111111", "hello_world", "en_US", nil); err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}

	// Meta rejects an empty components array, so the key has to be absent
	// rather than sent as [].
	want := `{"messaging_product":"whatsapp","to":"5215511111111","type":"template","template":{"name":"hello_world","language":{"code":"en_US"}}}`
	if got := stub.only(t).body; got != want {
		t.Errorf("body  = %s\nwant = %s", got, want)
	}
}

func TestSendTemplateCarriesBodyParametersInOrder(t *testing.T) {
	stub := newGraphStub(t, ok(`{"messages":[{"id":"wamid.A"}]}`))

	_, err := stub.client().SendTemplate(
		context.Background(), "555000111", "tok", "5215511111111",
		"appointment_reminder", "es", []string{"Rodrigo", "mañana a las 10:00"},
	)
	if err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}

	// Lowercase "body" on send; uppercase "BODY" on create. The asymmetry is
	// Meta's, and swapping either one is a 400.
	want := `{"messaging_product":"whatsapp","to":"5215511111111","type":"template",` +
		`"template":{"name":"appointment_reminder","language":{"code":"es"},` +
		`"components":[{"type":"body","parameters":[{"type":"text","text":"Rodrigo"},{"type":"text","text":"mañana a las 10:00"}]}]}}`
	if got := stub.only(t).body; got != want {
		t.Errorf("body  = %s\nwant = %s", got, want)
	}
}

func TestListTemplatesAsksForTheFieldsThePageRenders(t *testing.T) {
	stub := newGraphStub(t, ok(`{"data":[{"name":"appointment_reminder","status":"APPROVED","category":"UTILITY","language":"es","components":[{"type":"BODY","text":"Hola"}],"id":"123"}]}`))

	templates, err := stub.client().ListTemplates(context.Background(), "506463685879575", "tok")
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(templates))
	}
	// The elements pass through untouched, so a field Meta adds reaches the
	// page without a change here.
	if !strings.Contains(string(templates[0]), `"appointment_reminder"`) {
		t.Errorf("template = %s", templates[0])
	}

	call := stub.only(t)
	if call.path != "/v21.0/506463685879575/message_templates" {
		t.Errorf("path = %q", call.path)
	}
	if got := call.query["fields"]; len(got) != 1 || got[0] != "name,status,category,language,components" {
		t.Errorf("fields = %v", got)
	}
	if got := call.query["limit"]; len(got) != 1 || got[0] != "100" {
		t.Errorf("limit = %v, want [100]", got)
	}
}

func TestCreateTemplateSendsASingleUppercaseBodyComponent(t *testing.T) {
	stub := newGraphStub(t, ok(`{"id":"123456789012345","status":"PENDING","category":"UTILITY"}`))

	created, err := stub.client().CreateTemplate(context.Background(), "506463685879575", "tok", meta.TemplateDraft{
		Name:     "appointment_reminder",
		Category: "UTILITY",
		Language: "es",
		Body:     "Hola {{1}}, te recordamos tu cita {{2}}.",
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if !strings.Contains(string(created), `"PENDING"`) {
		t.Errorf("created = %s", created)
	}

	call := stub.only(t)
	if call.path != "/v21.0/506463685879575/message_templates" {
		t.Errorf("path = %q", call.path)
	}
	want := `{"name":"appointment_reminder","category":"UTILITY","language":"es","components":[{"type":"BODY","text":"Hola {{1}}, te recordamos tu cita {{2}}."}]}`
	if call.body != want {
		t.Errorf("body  = %s\nwant = %s", call.body, want)
	}
}

// The name clash is the failure a human hits most often, and Meta's own
// wording is more useful than anything this app could invent.
func TestCreateTemplateSurfacesMetasMessage(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusBadRequest, `{"error":{"message":"Template name already exists","code":100,"error_user_title":"Template Name Already Exists"}}`
	})

	_, err := stub.client().CreateTemplate(context.Background(), "waba", "tok", meta.TemplateDraft{Name: "x"})

	var apiErr *meta.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T, want *meta.APIError", err)
	}
	if apiErr.Message != "Template name already exists" {
		t.Errorf("Message = %q", apiErr.Message)
	}
}
