package gemini_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/gemini"
)

// capture is one request the client made, kept whole so a test can assert the
// wire shape rather than only the parsed result.
type capture struct {
	method string
	path   string
	query  string
	header http.Header
	body   map[string]any
}

// stub answers every call with the given status and body, and records the
// request. The client is pointed at it by setting BaseURL, which is the only
// seam the production code needs.
func stub(t *testing.T, status int, response string) (*gemini.Client, *capture) {
	t.Helper()

	got := &capture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.method, got.path, got.query, got.header = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		if err := json.Unmarshal(raw, &got.body); err != nil {
			t.Errorf("the request body is not JSON: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)

	client := gemini.New("test-api-key")
	client.BaseURL = server.URL
	client.Now = func() time.Time { return time.Date(2026, 10, 28, 19, 36, 0, 0, time.UTC) }

	return client, got
}

const oneCandidate = `{"candidates":[{"content":{"parts":[{"text":"¡Claro! ¿A qué hora?"}],"role":"model"},"finishReason":"STOP"}]}`

// The endpoint, the version and the auth style are not interchangeable, and a
// wrong one of any of them is invisible until a real customer messages a real
// number.
func TestSendsTheRequestTheSDKSends(t *testing.T) {
	client, got := stub(t, http.StatusOK, oneCandidate)

	if _, err := client.GenerateReply(context.Background(), nil, "hola"); err != nil {
		t.Fatalf("GenerateReply: %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %s, want POST", got.method)
	}
	if want := "/v1beta/models/gemini-2.5-flash:generateContent"; got.path != want {
		t.Errorf("path = %s, want %s", got.path, want)
	}
	// Non-streaming: the SDK adds ?alt=sse only for sendMessageStream.
	if got.query != "" {
		t.Errorf("query = %q, want none", got.query)
	}
	if key := got.header.Get("x-goog-api-key"); key != "test-api-key" {
		t.Errorf("x-goog-api-key = %q, want the configured key", key)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	// The key must not also travel in the URL, where it would be logged by
	// every proxy on the way.
	if strings.Contains(got.query, "key") {
		t.Errorf("the api key leaked into the query string: %q", got.query)
	}
}

// Verbatim from lib/ai-conversation.ts, including the blank third line and the
// em dash. The date is the Tijuana wall clock, which is the only thing the
// model is told about when "today" is.
const wantPrompt = `You are a friendly appointment booking assistant for a business.
Today is 10/28/2026, 12:36:00 PM.

Your job is to help customers book appointments through natural conversation.
Keep responses concise and conversational — this is WhatsApp, not email.
When the customer and business agree on a specific date and time, confirm the appointment clearly.
Reply in the same language the customer uses.`

func TestSendsTheSystemPromptVerbatim(t *testing.T) {
	client, got := stub(t, http.StatusOK, oneCandidate)

	if _, err := client.GenerateReply(context.Background(), nil, "hola"); err != nil {
		t.Fatalf("GenerateReply: %v", err)
	}

	instruction, _ := got.body["systemInstruction"].(map[string]any)
	if instruction == nil {
		t.Fatalf("no systemInstruction in %#v", got.body)
	}

	// role "user", not "system". The TypeScript passes the instruction to
	// startChat, which merges it without normalising, so this is what the live
	// model actually receives today.
	if role := instruction["role"]; role != "user" {
		t.Errorf("systemInstruction.role = %v, want \"user\"", role)
	}

	parts, _ := instruction["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("systemInstruction.parts = %#v, want one part", instruction["parts"])
	}
	if text, _ := parts[0].(map[string]any)["text"].(string); text != wantPrompt {
		t.Errorf("the system prompt differs from the one in production:\n got %q\nwant %q", text, wantPrompt)
	}
}

// The date is formatted the way JavaScript's toLocaleString('en-US') does it,
// because that is what the model has been seeing: no zero padding on the month
// or day, a comma before the time, and the Tijuana offset including whether
// daylight saving is in force.
func TestFormatsTodayLikeTheBrowserDid(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"daylight saving, october", time.Date(2026, 10, 28, 19, 36, 0, 0, time.UTC), "10/28/2026, 12:36:00 PM"},
		{"standard time, january", time.Date(2026, 1, 5, 20, 4, 5, 0, time.UTC), "1/5/2026, 12:04:05 PM"},
		{"midnight reads as 12 AM", time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC), "1/5/2026, 12:00:00 AM"},
		{"a single-digit hour", time.Date(2026, 7, 4, 16, 7, 9, 0, time.UTC), "7/4/2026, 9:07:09 AM"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, got := stub(t, http.StatusOK, oneCandidate)
			client.Now = func() time.Time { return tc.at }

			if _, err := client.GenerateReply(context.Background(), nil, "hola"); err != nil {
				t.Fatalf("GenerateReply: %v", err)
			}

			instruction, _ := got.body["systemInstruction"].(map[string]any)
			parts, _ := instruction["parts"].([]any)
			text, _ := parts[0].(map[string]any)["text"].(string)

			if want := "Today is " + tc.want + "."; !strings.Contains(text, want) {
				t.Errorf("the prompt does not contain %q:\n%s", want, text)
			}
		})
	}
}

func TestSendsTheHistoryThenTheNewTurn(t *testing.T) {
	client, got := stub(t, http.StatusOK, oneCandidate)

	history := []gemini.Message{
		{Role: gemini.RoleUser, Content: "hola"},
		{Role: gemini.RoleModel, Content: "¡Hola! ¿En qué te puedo ayudar?"},
	}
	if _, err := client.GenerateReply(context.Background(), history, "quiero una cita el martes"); err != nil {
		t.Fatalf("GenerateReply: %v", err)
	}

	want := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hola"}}},
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "¡Hola! ¿En qué te puedo ayudar?"}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "quiero una cita el martes"}}},
	}
	assertJSONEqual(t, "contents", got.body["contents"], want)

	// Both are sent empty rather than omitted, matching the SDK. An absent
	// generationConfig and an absent safetySettings mean the same thing to the
	// API, but keeping the request byte-comparable removes a variable when a
	// live call misbehaves.
	assertJSONEqual(t, "generationConfig", got.body["generationConfig"], map[string]any{})
	assertJSONEqual(t, "safetySettings", got.body["safetySettings"], []any{})
}

// A conversation the business started — the first row is an echo of their own
// message — is the case the SDK refused to send at all, which left the
// customer with no reply. Nothing here refuses it.
func TestAcceptsAHistoryThatStartsWithTheBusiness(t *testing.T) {
	client, got := stub(t, http.StatusOK, oneCandidate)

	history := []gemini.Message{{Role: gemini.RoleModel, Content: "¿Sigue en pie su cita?"}}
	reply, err := client.GenerateReply(context.Background(), history, "sí, ahí estaré")
	if err != nil {
		t.Fatalf("GenerateReply: %v", err)
	}
	if reply == "" {
		t.Error("no reply was generated for a conversation the business started")
	}

	contents, _ := got.body["contents"].([]any)
	if len(contents) != 2 {
		t.Fatalf("contents = %#v, want the model turn and the new user turn", got.body["contents"])
	}
	if role, _ := contents[0].(map[string]any)["role"]; role != "model" {
		t.Errorf("the leading turn was rewritten to %v; it should be sent as stored", role)
	}
}

func TestReadsTheReply(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		want     string
	}{
		{"one part", oneCandidate, "¡Claro! ¿A qué hora?"},
		{
			// getText concatenates with no separator.
			"several parts",
			`{"candidates":[{"content":{"parts":[{"text":"uno "},{"text":"dos"}],"role":"model"},"finishReason":"STOP"}]}`,
			"uno dos",
		},
		{
			// MAX_TOKENS is a truncated answer, not a refused one, and the SDK
			// hands it back rather than throwing.
			"a truncated answer",
			`{"candidates":[{"content":{"parts":[{"text":"te puedo ofrecer"}],"role":"model"},"finishReason":"MAX_TOKENS"}]}`,
			"te puedo ofrecer",
		},
		{
			// More than one candidate: the first is the answer.
			"several candidates",
			`{"candidates":[{"content":{"parts":[{"text":"primero"}]}},{"content":{"parts":[{"text":"segundo"}]}}]}`,
			"primero",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := stub(t, http.StatusOK, tc.response)

			got, err := client.GenerateReply(context.Background(), nil, "hola")
			if err != nil {
				t.Fatalf("GenerateReply: %v", err)
			}
			if got != tc.want {
				t.Errorf("reply = %q, want %q", got, tc.want)
			}
		})
	}
}

// Every one of these ends in no reply being sent. They are errors rather than
// an empty string because an empty string would be handed to WhatsApp as a
// message body, which Meta rejects with an error about the wrong thing.
func TestReportsAnswersItCannotUse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		response string
		contains string
	}{
		{
			"a blocked prompt",
			http.StatusOK,
			`{"promptFeedback":{"blockReason":"SAFETY"}}`,
			"SAFETY",
		},
		{
			"a candidate blocked for safety",
			http.StatusOK,
			`{"candidates":[{"finishReason":"SAFETY","content":{"parts":[]}}]}`,
			"SAFETY",
		},
		{
			"an answer with no candidates at all",
			http.StatusOK,
			`{}`,
			"no reply",
		},
		{
			"an answer whose parts carry no text",
			http.StatusOK,
			`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png"}}]},"finishReason":"STOP"}]}`,
			"no reply",
		},
		{
			"a quota failure",
			http.StatusTooManyRequests,
			`{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`,
			"Resource has been exhausted",
		},
		{
			"a failure with no JSON body",
			http.StatusBadGateway,
			`<html>502</html>`,
			"502",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := stub(t, tc.status, tc.response)

			reply, err := client.GenerateReply(context.Background(), nil, "hola")
			if err == nil {
				t.Fatalf("GenerateReply returned %q and no error", reply)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error = %q, want it to mention %q", err, tc.contains)
			}
		})
	}
}

// A hung Gemini connection must not hold a webhook delivery open: the router
// is waiting on the other end with its own timeout, and a reply that arrives
// after it has given up is a reply nobody reads.
func TestTheClientHasATimeout(t *testing.T) {
	client := gemini.New("test-api-key")

	if client.HTTP == nil || client.HTTP.Timeout == 0 {
		t.Error("the default client has no timeout")
	}
}

func assertJSONEqual(t *testing.T, label string, got, want any) {
	t.Helper()

	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("%s = %s, want %s", label, gotJSON, wantJSON)
	}
}
