package chat_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/Rodriamarog/instant_appoint/internal/chat"
	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// The send route is the one piece of the chat page that cannot live in the
// browser: it spends a connected account's Meta access token. Everything below
// drives it through the real route registration, with only Meta stubbed.

// ownerToken is the access token the stubbed account holds. No response may
// contain it.
const ownerToken = "EAAG-owner-token-must-not-leak"

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
	user.SetPassword("devpassword123")
	user.SetVerified(true)
	if err := app.Save(user); err != nil {
		t.Fatalf("Save user: %v", err)
	}

	return user
}

// graph stands in for Meta's Graph API and records every request, so a test
// can assert which number sent and with whose token.
type graph struct {
	server *httptest.Server

	requests []graphRequest

	// reply is the body a send is answered with, and status its code. The
	// %d in a reply is filled with the call number, because Meta issues a
	// fresh wamid per message and the deduplication index refuses a repeat.
	reply  string
	status int
}

type graphRequest struct {
	method string
	path   string
	bearer string
	body   string
}

func newGraph(t *testing.T) *graph {
	t.Helper()

	g := &graph{
		reply:  `{"messaging_product":"whatsapp","messages":[{"id":"wamid.HUMAN_REPLY_%d"}]}`,
		status: http.StatusOK,
	}

	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := len(g.requests)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read stub request body: %v", err)
		}

		g.requests = append(g.requests, graphRequest{
			method: r.Method,
			path:   r.URL.Path,
			bearer: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
			body:   string(body),
		})

		payload := g.reply
		if strings.Contains(payload, "%d") {
			payload = fmt.Sprintf(payload, call)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(g.status)
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Errorf("write stub response: %v", err)
		}
	}))
	t.Cleanup(g.server.Close)

	return g
}

func (g *graph) client() *meta.Client {
	c := meta.New("APPID", "APPSECRET")
	c.BaseURL = g.server.URL

	return c
}

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

	chat.Register(r, client)

	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	return mux
}

// account gives userID a connected Cloud API account on one number.
func account(t *testing.T, app core.App, userID, phoneNumberID, token string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(migrations.WhatsAppAccountsCollection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(whatsapp_accounts): %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("session_id", "business_"+userID+"_"+phoneNumberID)
	record.Set("account_type", "business_api")
	record.Set("waba_id", "100000000000001")
	record.Set("phone_number_id", phoneNumberID)
	record.Set("access_token", token)
	record.Set("status", "connected")
	record.Set("is_active", true)
	if err := app.Save(record); err != nil {
		t.Fatalf("Save whatsapp account: %v", err)
	}

	return record
}

// conversation writes a customer thread directly, the way a delivery would
// have left one.
func conversation(t *testing.T, app core.App, userID, customerPhone, phoneNumberID string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(migrations.ConversationsCollection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(conversations): %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("customer_phone", customerPhone)
	record.Set("status", "active")
	record.Set("phone_number_id", phoneNumberID)
	record.Set("last_message_at", "2026-10-29T12:00:00.000Z")
	if err := app.Save(record); err != nil {
		t.Fatalf("Save conversation: %v", err)
	}

	return record
}

func send(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat/send", reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body, err)
	}

	return body
}

// messagesIn returns every stored message in a conversation, oldest first.
func messagesIn(t *testing.T, app core.App, conversationID string) []*core.Record {
	t.Helper()

	records, err := app.FindRecordsByFilter(
		migrations.WhatsAppMessagesCollection,
		"conversation_id = {:id}", "sent_at", 0, 0,
		map[string]any{"id": conversationID},
	)
	if err != nil {
		t.Fatalf("read stored messages: %v", err)
	}

	return records
}

// The whole happy path: the reply goes out from the number the customer has
// been talking to, with that account's token, and is stored as an outbound
// message in the conversation.
func TestAReplyIsSentAndStored(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, user.Id, "5216641234567", "200000000000002")
	g := newGraph(t)
	mux := newMux(t, app, g.client(), user)

	rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"On my way."}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	if len(g.requests) != 1 {
		t.Fatalf("made %d Graph calls, want 1: %v", len(g.requests), g.requests)
	}
	got := g.requests[0]
	if got.path != "/v21.0/200000000000002/messages" {
		t.Errorf("path = %q, want the conversation's own number at v21.0", got.path)
	}
	if got.bearer != ownerToken {
		t.Errorf("bearer = %q, want that account's access token", got.bearer)
	}
	if !strings.Contains(got.body, `"to":"5216641234567"`) {
		t.Errorf("body = %s, want it addressed to the customer", got.body)
	}
	if !strings.Contains(got.body, `"body":"On my way."`) {
		t.Errorf("body = %s, want it to carry the typed text", got.body)
	}

	stored := messagesIn(t, app, conv.Id)
	if len(stored) != 1 {
		t.Fatalf("stored %d messages, want 1", len(stored))
	}
	message := stored[0]
	for field, want := range map[string]string{
		"user_id":         user.Id,
		"to_number":       "5216641234567",
		"message_content": "On my way.",
		"message_type":    "text",
		"direction":       "outbound",
		"conversation_id": conv.Id,
		"message_id":      "wamid.HUMAN_REPLY_0",
	} {
		if value := message.GetString(field); value != want {
			t.Errorf("stored %s = %q, want %q", field, value, want)
		}
	}

	if strings.Contains(rec.Body.String(), ownerToken) {
		t.Errorf("the response leaks the account access token: %s", rec.Body)
	}
}
