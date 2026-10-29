// Package meta is this app's client for Meta's Graph API.
//
// Every call here was transcribed from the TypeScript that calls these
// endpoints successfully today, because Meta's API is only as documented as
// the request that last worked. Two consequences are worth knowing before
// changing anything:
//
//   - The API version is per call, not per client. The token exchange, the
//     webhook subscription and the phone-number lookup are v25.0; message
//     sends and the template calls are v21.0; the verified-name lookup is
//     v21.0. They are not interchangeable.
//   - So is the auth style. Most calls carry an Authorization: Bearer header,
//     but the verified-name lookup passes ?access_token= as a query parameter
//     and the token exchange passes ?client_secret=.
//
// A wrong version or auth style stays invisible until a human runs the real
// signup, so the tests assert the literal wire shape rather than only the
// parsed result.
package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GraphBaseURL is the production Graph API host. Tests point Client.BaseURL at
// an httptest server instead.
const GraphBaseURL = "https://graph.facebook.com"

// requestTimeout bounds every call. The TypeScript set none, so a hung Meta
// connection held a request open until the platform killed it.
const requestTimeout = 10 * time.Second

// Client talks to the Graph API on behalf of one app.
//
// The access tokens belong to the connected accounts rather than to the app,
// so they are passed per call rather than held here. Only the app's own
// credentials, which are needed to turn a signup code into a token, live on
// the client.
type Client struct {
	// BaseURL is the Graph host, without a trailing slash or a version.
	BaseURL string

	// HTTP is the transport. It has a timeout; replacing it without one
	// reintroduces the hang this package exists to bound.
	HTTP *http.Client

	appID     string
	appSecret string
}

// New returns a client for the given Meta app credentials.
func New(appID, appSecret string) *Client {
	return &Client{
		BaseURL:   GraphBaseURL,
		HTTP:      &http.Client{Timeout: requestTimeout},
		appID:     appID,
		appSecret: appSecret,
	}
}

// APIError is an error Meta reported, whether it arrived with a failing status
// code or inside a 200.
type APIError struct {
	// Status is the HTTP status the response carried.
	Status int

	// Message is Meta's own error.message, which is the only part fit to show
	// a human.
	Message string

	// Code and Subcode identify the failure. 100/36009 is a reused signup
	// code, 190 an expired token, 131030 a recipient outside the allow list.
	Code    int
	Subcode int

	// Body is the whole response, kept because Meta puts failures in places
	// the typed fields do not reach.
	Body string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("meta: %s (http %d, code %d)", e.Message, e.Status, e.Code)
	}

	return fmt.Sprintf("meta: request failed (http %d): %s", e.Status, e.Body)
}

// Safe is the whole of this failure that may leave the process: Meta's own
// explanation and the codes, never Body.
//
// Body is kept on the error because Meta puts failures in places the typed
// fields do not reach, but it is also a verbatim echo of the request — which
// for several of these calls quotes the app secret in the query string. The
// default Error() prints it whenever Meta explained nothing, so anything
// carrying one of these to a log line or an HTTP response goes through here
// instead. This is the one place that rule is written down.
func (e *APIError) Safe() string {
	message := e.Message
	if message == "" {
		message = "Meta refused the request and explained nothing"
	}

	return fmt.Sprintf("%s (http %d, code %d/%d)", message, e.Status, e.Code, e.Subcode)
}

// Explain returns Meta's own sentence about err, and whether there was one.
//
// This is the part written for a person — "Message failed to send because more
// than 24 hours have passed" is the whole answer to a refused reply — and the
// only part a caller may put in front of one. Everything else about the
// failure, the body above all, stays here.
func Explain(err error) (string, bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message == "" {
		return "", false
	}

	return apiErr.Message, true
}

// Sanitise returns err with Meta's raw response body left out, for a failure
// on its way somewhere it should not travel whole — a log line, a wrapped
// return value. An error that is not a Meta failure comes back unchanged.
func Sanitise(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	return errors.New("meta: " + apiErr.Safe())
}

// errorEnvelope is the shape every Graph error arrives in.
type errorEnvelope struct {
	Error *struct {
		Message   string `json:"message"`
		Type      string `json:"type"`
		Code      int    `json:"code"`
		Subcode   int    `json:"error_subcode"`
		TraceID   string `json:"fbtrace_id"`
		UserTitle string `json:"error_user_title"`
		UserMsg   string `json:"error_user_msg"`
	} `json:"error"`
}

// do performs req and returns the response body, turning both a failing status
// code and an error body on a 200 into an *APIError.
func (c *Client) do(req *http.Request) ([]byte, error) {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("meta: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close() //nolint:errcheck // nothing useful to do with a close error on a read

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("meta: read %s %s: %w", req.Method, req.URL.Path, err)
	}

	var envelope errorEnvelope
	// A body that is not JSON at all is itself a failure worth reporting, but
	// only when the status says so: a successful call with a surprising body
	// is the caller's problem to parse.
	_ = json.Unmarshal(body, &envelope)

	if res.StatusCode >= http.StatusBadRequest || envelope.Error != nil {
		apiErr := &APIError{Status: res.StatusCode, Body: string(body)}
		if envelope.Error != nil {
			apiErr.Message = envelope.Error.Message
			apiErr.Code = envelope.Error.Code
			apiErr.Subcode = envelope.Error.Subcode
		}

		return body, apiErr
	}

	return body, nil
}

// get builds a GET for the given version and path.
func (c *Client) get(ctx context.Context, version, path string, query url.Values, bearer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(version, path, query), nil)
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	return c.do(req)
}

// post builds a POST. A nil payload sends no body and no Content-Type, which
// is what the subscribed_apps call expects.
func (c *Client) post(ctx context.Context, version, path string, payload any, bearer string) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := marshal(payload)
		if err != nil {
			return nil, err
		}
		body = strings.NewReader(string(encoded))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(version, path, nil), body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	return c.do(req)
}

func (c *Client) url(version, path string, query url.Values) string {
	base := c.BaseURL
	if base == "" {
		base = GraphBaseURL
	}

	full := strings.TrimSuffix(base, "/") + "/" + version + "/" + strings.TrimPrefix(path, "/")
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	return full
}

// ExchangeCode turns an Embedded Signup authorization code into an access
// token for the account that just finished signing up.
//
// The token is long-lived and is never refreshed: Meta issues it once and this
// app stores it as-is.
func (c *Client) ExchangeCode(ctx context.Context, code string) (string, error) {
	body, err := c.get(ctx, "v25.0", "oauth/access_token", url.Values{
		"client_id":     {c.appID},
		"client_secret": {c.appSecret},
		"code":          {code},
	}, "")
	if err != nil {
		return "", err
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("meta: parse token exchange response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", &APIError{Status: http.StatusOK, Message: "token exchange returned no access_token", Body: string(body)}
	}

	return parsed.AccessToken, nil
}

// ErrPhoneNumberUnresolved means Meta did not say which number was onboarded
// and the WABA does not have exactly one, so there is nothing to pick.
//
// It is deliberately distinct from an *APIError: a WABA with two numbers is a
// signup this app cannot complete, while a rejected token is a different
// problem with a different fix.
var ErrPhoneNumberUnresolved = errors.New("meta: cannot tell which phone number was onboarded")

// ResolvePhoneNumberID returns the phone number the signup onboarded.
//
// phoneNumberID is what the signup reported, which may be empty: a business
// that keeps using the WhatsApp Business app finishes with
// FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING and Meta sends no number. That is
// the common case, not an edge case, and the fallback below — the WABA's only
// number — is what makes it work.
func (c *Client) ResolvePhoneNumberID(ctx context.Context, wabaID, phoneNumberID, accessToken string) (string, error) {
	if phoneNumberID != "" {
		return phoneNumberID, nil
	}

	body, err := c.get(ctx, "v25.0", wabaID+"/phone_numbers", url.Values{"fields": {"id"}}, accessToken)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Data []struct {
			// Meta sends ids as JSON strings; anything else is not an id this
			// app can use, so it is dropped rather than coerced.
			ID json.RawMessage `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("meta: parse phone_numbers response: %w", err)
	}

	var ids []string
	for _, number := range parsed.Data {
		var id string
		if err := json.Unmarshal(number.ID, &id); err == nil && id != "" {
			ids = append(ids, id)
		}
	}

	// Two numbers is as unusable as none: picking one would attach the
	// account to a number the business did not onboard.
	if len(ids) != 1 {
		return "", fmt.Errorf("%w: the WABA reported %d numbers", ErrPhoneNumberUnresolved, len(ids))
	}

	return ids[0], nil
}

// SubscribeApp subscribes this app to the WABA's webhook events.
//
// Without it Meta accepts the connection but delivers nothing, so inbound
// messages never reach the webhook and the account looks connected while being
// deaf. The call takes no body: adding one makes Meta reject it.
func (c *Client) SubscribeApp(ctx context.Context, wabaID, accessToken string) error {
	body, err := c.post(ctx, "v25.0", wabaID+"/subscribed_apps", nil, accessToken)
	if err != nil {
		return err
	}

	var parsed struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("meta: parse subscribed_apps response: %w", err)
	}

	// A refusal arrives as 200 with success:false, so the body is the only
	// thing that says whether this worked.
	if !parsed.Success {
		return &APIError{Status: http.StatusOK, Message: "the WABA webhook subscription was not confirmed", Body: string(body)}
	}

	return nil
}

// PhoneNumberVerifiedName returns Meta's display name for a phone number — the
// business name shown next to the number in WhatsApp.
//
// It asks for display_phone_number too, matching the call this was ported
// from, because the response is cheap and a later ticket may want the number
// Meta formats for display.
func (c *Client) PhoneNumberVerifiedName(ctx context.Context, phoneNumberID, accessToken string) (string, error) {
	// v21.0, and the token goes in the query string: this is the one call in
	// the app that does not use a Bearer header, and Meta rejects it with 400
	// if the token is moved to a header.
	body, err := c.get(ctx, "v21.0", phoneNumberID, url.Values{
		"fields":       {"verified_name,display_phone_number"},
		"access_token": {accessToken},
	}, "")
	if err != nil {
		return "", err
	}

	var parsed struct {
		VerifiedName string `json:"verified_name"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("meta: parse phone number response: %w", err)
	}

	return parsed.VerifiedName, nil
}

// marshal encodes a request payload the way JSON.stringify does.
//
// encoding/json escapes <, > and & to < and friends by default. That is
// still valid JSON and Meta decodes it correctly, but it makes a message body
// unreadable in a log and in a captured request, which is where these payloads
// are usually debugged.
func marshal(payload any) ([]byte, error) {
	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}

	// Encode appends a newline; JSON.stringify does not.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// sendResult is the part of a send response this app reads. Meta also returns
// a contacts array, which nothing uses.
type sendResult struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// messageID pulls the WhatsApp message id out of a send response.
//
// The TypeScript returned "" when it was missing and wrote that empty string
// to whatsapp_messages.message_id, which is the webhook's deduplication key —
// so a missing id silently disabled dedup for that message. This is the
// deliberate change: no id is an error.
func messageID(body []byte) (string, error) {
	var parsed sendResult
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("meta: parse send response: %w", err)
	}
	if len(parsed.Messages) == 0 || parsed.Messages[0].ID == "" {
		return "", &APIError{Status: http.StatusOK, Message: "the send response carried no message id", Body: string(body)}
	}

	return parsed.Messages[0].ID, nil
}

// The send payloads are structs rather than maps so the keys serialize in the
// order Meta is known to accept. JSON objects are unordered in principle, but
// these are the exact requests the working implementation sends, and keeping
// them byte-identical removes one variable from debugging a live failure.
type sendRequest struct {
	MessagingProduct string        `json:"messaging_product"`
	To               string        `json:"to"`
	Type             string        `json:"type"`
	Text             *textBody     `json:"text,omitempty"`
	Template         *templateBody `json:"template,omitempty"`
}

type textBody struct {
	Body string `json:"body"`
}

type templateBody struct {
	Name     string `json:"name"`
	Language struct {
		Code string `json:"code"`
	} `json:"language"`
	Components []templateComponent `json:"components,omitempty"`
}

type templateComponent struct {
	Type       string              `json:"type"`
	Parameters []templateParameter `json:"parameters"`
}

type templateParameter struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SendText sends a plain text message and returns its WhatsApp message id.
//
// to is digits including the country code and without a leading +, which is
// the form both the webhook and the stored customer phone numbers use.
func (c *Client) SendText(ctx context.Context, phoneNumberID, accessToken, to, text string) (string, error) {
	body, err := c.post(ctx, "v21.0", phoneNumberID+"/messages", sendRequest{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "text",
		Text:             &textBody{Body: text},
	}, accessToken)
	if err != nil {
		return "", err
	}

	return messageID(body)
}

// SendTemplate sends an approved message template and returns its WhatsApp
// message id.
//
// Outside the 24-hour customer service window this is the only kind of message
// Meta accepts, which is why reminders go out as templates.
func (c *Client) SendTemplate(ctx context.Context, phoneNumberID, accessToken, to, name, languageCode string, bodyParameters []string) (string, error) {
	template := &templateBody{Name: name}
	template.Language.Code = languageCode

	// Meta rejects an empty components array, so a template without
	// parameters must omit the key rather than send [].
	if len(bodyParameters) > 0 {
		parameters := make([]templateParameter, 0, len(bodyParameters))
		for _, text := range bodyParameters {
			parameters = append(parameters, templateParameter{Type: "text", Text: text})
		}
		// Lowercase "body" on send. Create uses uppercase "BODY".
		template.Components = []templateComponent{{Type: "body", Parameters: parameters}}
	}

	body, err := c.post(ctx, "v21.0", phoneNumberID+"/messages", sendRequest{
		MessagingProduct: "whatsapp",
		To:               to,
		Type:             "template",
		Template:         template,
	}, accessToken)
	if err != nil {
		return "", err
	}

	return messageID(body)
}

// ListTemplates returns the WABA's message templates, unparsed.
//
// Each element is passed through as Meta sent it: the page renders name,
// status, category, language and components, and a field Meta adds later
// arrives without a change here. More than 100 templates are truncated, which
// matches the behaviour this was ported from.
func (c *Client) ListTemplates(ctx context.Context, wabaID, accessToken string) ([]json.RawMessage, error) {
	body, err := c.get(ctx, "v21.0", wabaID+"/message_templates", url.Values{
		"fields": {"name,status,category,language,components"},
		"limit":  {"100"},
	}, accessToken)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("meta: parse message_templates response: %w", err)
	}

	return parsed.Data, nil
}

// TemplateDraft is a template submitted for Meta's approval.
//
// Only a body is supported, matching what the UI can create: no header,
// footer, buttons or example values.
type TemplateDraft struct {
	// Name is lowercase with underscores; Meta rejects anything else.
	Name string

	// Category is UTILITY, MARKETING or AUTHENTICATION, and decides both the
	// approval bar and the price.
	Category string

	// Language is a Meta language code such as es or en_US.
	Language string

	// Body may contain {{1}}-style placeholders, filled in by the
	// bodyParameters passed to SendTemplate.
	Body string
}

// CreateTemplate submits a template for approval and returns Meta's response
// unparsed; a newly created template is PENDING, not usable yet.
func (c *Client) CreateTemplate(ctx context.Context, wabaID, accessToken string, draft TemplateDraft) (json.RawMessage, error) {
	body, err := c.post(ctx, "v21.0", wabaID+"/message_templates", createTemplateRequest{
		Name:     draft.Name,
		Category: draft.Category,
		Language: draft.Language,
		// Uppercase "BODY" on create, lowercase "body" on send.
		Components: []createComponent{{Type: "BODY", Text: draft.Body}},
	}, accessToken)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(body), nil
}

type createTemplateRequest struct {
	Name       string            `json:"name"`
	Category   string            `json:"category"`
	Language   string            `json:"language"`
	Components []createComponent `json:"components"`
}

type createComponent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
