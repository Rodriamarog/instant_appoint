// Package gemini asks Google's Gemini to write the next turn of a WhatsApp
// conversation.
//
// It speaks to the REST API over net/http rather than through
// @google/generative-ai, which is what the TypeScript used. The SDK's only
// contributions were building this request and reading this response, and it
// brought a dependency tree and a layer of defaults that had to be reverse
// engineered anyway to know what was going on the wire. The request below is
// that reverse engineering, written out.
//
// Two places where this deliberately differs from the SDK, both of which
// change what a customer receives:
//
//   - The SDK refused to send a conversation whose oldest turn was the
//     business rather than the customer — a validation of its own, not the
//     API's. A business that messages a customer first from the WhatsApp
//     Business app produces exactly that history, and the customer's reply
//     then went unanswered. Such a history is sent here as it stands.
//   - An answer with no usable text is an error here rather than an empty
//     string. The empty string was passed on to WhatsApp as a message body,
//     which Meta rejects, so the failure surfaced as a confusing Meta error
//     instead of the Gemini one that caused it.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	// A pure-Go binary carries no system time zone database, and without this
	// the business's wall clock below silently becomes UTC — a date the model
	// would then quote back to customers.
	_ "time/tzdata"
)

// BaseURL is the production API host. Tests point Client.BaseURL at an
// httptest server instead.
const BaseURL = "https://generativelanguage.googleapis.com"

// The model and API version, which travel together: the path is
// /<version>/models/<model>:generateContent.
const (
	Model      = "gemini-2.5-flash"
	apiVersion = "v1beta"
)

// requestTimeout bounds a call. The TypeScript set none, so a hung connection
// held a webhook delivery open until the platform killed it.
const requestTimeout = 30 * time.Second

// businessZone is the time zone the "today is" line is written in. The
// business and its customers are in Tijuana; the model is told nothing else
// about time, so this one line decides what "tomorrow at 3" means.
const businessZone = "America/Tijuana"

// jsDateTime is Go's spelling of what JavaScript's
// toLocaleString('en-US') produces: no zero padding on the month, day or
// hour, a comma between date and time, and a 12-hour clock with AM/PM.
const jsDateTime = "1/2/2006, 3:04:05 PM"

// Roles, as the API spells them. A conversation is told to the model as the
// customer's turns and the business's turns.
const (
	RoleUser  = "user"
	RoleModel = "model"
)

// Message is one turn of a conversation.
type Message struct {
	// Role is RoleUser or RoleModel.
	Role    string
	Content string
}

// Client generates replies with one API key.
type Client struct {
	// BaseURL is the API host, without a trailing slash or a version.
	BaseURL string

	// HTTP is the transport. It has a timeout; replacing it without one
	// reintroduces the hang this package bounds.
	HTTP *http.Client

	// Now is the clock the "today is" line is read from. Tests freeze it.
	Now func() time.Time

	apiKey string
	zone   *time.Location
}

// New returns a client for the given API key.
func New(apiKey string) *Client {
	// With tzdata compiled in above this cannot fail. If it somehow does, UTC
	// keeps replies flowing with a date that is a few hours out, which is less
	// bad than no replies at all.
	zone, err := time.LoadLocation(businessZone)
	if err != nil {
		zone = time.UTC
	}

	return &Client{
		BaseURL: BaseURL,
		HTTP:    &http.Client{Timeout: requestTimeout},
		Now:     time.Now,
		apiKey:  apiKey,
		zone:    zone,
	}
}

// GenerateReply writes the business's answer to incoming, given the
// conversation so far.
//
// history is oldest first and must not contain incoming: it is the context,
// and incoming is the turn being answered.
func (c *Client) GenerateReply(ctx context.Context, history []Message, incoming string) (string, error) {
	contents := make([]content, 0, len(history)+1)
	for _, message := range history {
		contents = append(contents, content{Role: message.Role, Parts: []part{{Text: message.Content}}})
	}
	contents = append(contents, content{Role: RoleUser, Parts: []part{{Text: incoming}}})

	payload, err := json.Marshal(request{
		// Both are sent empty rather than omitted, which is what the SDK did.
		// They mean the same thing to the API; keeping the request identical
		// removes a variable when a live call misbehaves.
		SafetySettings:   []any{},
		GenerationConfig: map[string]any{},
		SystemInstruction: &content{
			// "user", not "system". The TypeScript passed the instruction to
			// startChat, which merged it in without normalising the role, so
			// this is what the live model has been receiving. Changing it to
			// "system" would be a prompt change dressed up as a port.
			Role:  RoleUser,
			Parts: []part{{Text: c.systemPrompt()}},
		},
		Contents: contents,
	})
	if err != nil {
		return "", fmt.Errorf("gemini: encode the request: %w", err)
	}

	url := fmt.Sprintf("%s/%s/models/%s:generateContent", strings.TrimSuffix(c.baseURL(), "/"), apiVersion, Model)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("gemini: build the request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	// A header rather than ?key=, so the credential is not written into every
	// access log and proxy trace between here and Google.
	httpRequest.Header.Set("x-goog-api-key", c.apiKey)

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}

	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}
	defer httpResponse.Body.Close() //nolint:errcheck // nothing useful to do with a close error on a read

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return "", fmt.Errorf("gemini: read the response: %w", err)
	}

	if httpResponse.StatusCode >= http.StatusBadRequest {
		return "", apiError(httpResponse.StatusCode, body)
	}

	return replyText(body)
}

func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return BaseURL
	}

	return c.BaseURL
}

// systemPrompt is the instruction from lib/ai-conversation.ts, verbatim.
//
// The blank line and the em dash are part of it. The date is the only variable
// and the only thing anchoring the model's sense of when "next Tuesday" is.
func (c *Client) systemPrompt() string {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}

	return "You are a friendly appointment booking assistant for a business.\n" +
		"Today is " + now().In(c.zone).Format(jsDateTime) + ".\n" +
		"\n" +
		"Your job is to help customers book appointments through natural conversation.\n" +
		"Keep responses concise and conversational — this is WhatsApp, not email.\n" +
		"When the customer and business agree on a specific date and time, confirm the appointment clearly.\n" +
		"Reply in the same language the customer uses."
}

// --- wire shapes ---------------------------------------------------------

type request struct {
	SafetySettings    []any          `json:"safetySettings"`
	GenerationConfig  map[string]any `json:"generationConfig"`
	SystemInstruction *content       `json:"systemInstruction,omitempty"`
	Contents          []content      `json:"contents"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type response struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`

	// PromptFeedback is present instead of candidates when the prompt itself
	// was refused.
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// blockedFinishReasons are the ways an answer can come back refused rather
// than written. MAX_TOKENS and STOP are not among them: a truncated answer is
// still an answer.
var blockedFinishReasons = map[string]bool{
	"SAFETY":             true,
	"RECITATION":         true,
	"LANGUAGE":           true,
	"BLOCKLIST":          true,
	"PROHIBITED_CONTENT": true,
	"SPII":               true,
}

// replyText pulls the answer out of a successful response, or explains why
// there is none.
func replyText(body []byte) (string, error) {
	var parsed response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("gemini: parse the response: %w", err)
	}

	if len(parsed.Candidates) == 0 {
		if parsed.PromptFeedback != nil && parsed.PromptFeedback.BlockReason != "" {
			return "", fmt.Errorf("gemini: the prompt was blocked (%s)", parsed.PromptFeedback.BlockReason)
		}

		return "", fmt.Errorf("gemini: the answer carried no reply")
	}

	// The first candidate is the answer; the API returns one unless asked for
	// more, and nothing here asks.
	candidate := parsed.Candidates[0]
	if blockedFinishReasons[candidate.FinishReason] {
		return "", fmt.Errorf("gemini: the answer was blocked (%s)", candidate.FinishReason)
	}

	var reply strings.Builder
	for _, p := range candidate.Content.Parts {
		reply.WriteString(p.Text)
	}
	if reply.Len() == 0 {
		// An empty string here used to be sent to WhatsApp as a message body
		// and rejected by Meta, which reported it as a Meta problem.
		return "", fmt.Errorf("gemini: the answer carried no reply text (finish reason %q)", candidate.FinishReason)
	}

	return reply.String(), nil
}

// apiError turns a failing response into the error worth logging: Google's own
// message when there is one, the raw body when there is not.
func apiError(status int, body []byte) error {
	var envelope struct {
		Error *struct {
			Message string          `json:"message"`
			Status  string          `json:"status"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)

	if envelope.Error != nil && envelope.Error.Message != "" {
		if len(envelope.Error.Details) > 0 {
			return fmt.Errorf("gemini: %s (http %d): %s %s", envelope.Error.Status, status, envelope.Error.Message, envelope.Error.Details)
		}

		return fmt.Errorf("gemini: %s (http %d): %s", envelope.Error.Status, status, envelope.Error.Message)
	}

	return fmt.Errorf("gemini: request failed (http %d): %s", status, truncate(string(body), 500))
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}

	return value[:max] + "…"
}
