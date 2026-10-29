package chat_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/migrations"
)

// The chat page reads its conversations and its thread from PocketBase
// directly — there is no handler in front of them — so the collections' access
// rules are the only thing keeping one business's customers away from
// another's. Everything below goes through the real record API, with the same
// filters and sorts web/src/lib/conversations.ts and messages.ts send, because
// that is the layer the rules are evaluated at.

// recordAPI builds PocketBase's own HTTP API on top of a migrated database.
func recordAPI(t *testing.T, app core.App) http.Handler {
	t.Helper()

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("apis.NewRouter: %v", err)
	}

	mux, err := pbRouter.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	return mux
}

// tokenFor is the auth token that person's browser would hold.
func tokenFor(t *testing.T, user *core.Record) string {
	t.Helper()

	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	return token
}

// listed issues one authenticated list request and returns the items.
func listed(t *testing.T, mux http.Handler, token, collection, filter, sort string) []map[string]any {
	t.Helper()

	query := url.Values{"sort": {sort}, "perPage": {"200"}}
	if filter != "" {
		query.Set("filter", filter)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+collection+"/records?"+query.Encode(), nil)
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200 (body: %s)", collection, rec.Code, rec.Body)
	}

	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("body %q is not a record page: %v", rec.Body, err)
	}

	return page.Items
}

// message writes one message into a conversation directly.
func message(t *testing.T, app core.App, conv *core.Record, direction, content, sentAt string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(migrations.WhatsAppMessagesCollection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(whatsapp_messages): %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", conv.GetString("user_id"))
	record.Set("to_number", conv.GetString("customer_phone"))
	record.Set("message_content", content)
	record.Set("message_type", "text")
	record.Set("direction", direction)
	record.Set("conversation_id", conv.Id)
	record.Set("sent_at", sentAt)
	if err := app.Save(record); err != nil {
		t.Fatalf("Save message: %v", err)
	}

	return record
}

// The conversation list is this person's own, most recently active first.
// last_message_at is a text column, so "most recent first" is a descending
// string sort and only holds while every writer emits the same fixed width.
func TestTheConversationListIsYourOwnMostRecentFirst(t *testing.T) {
	app := newApp(t)
	owner := newUser(t, app, "owner@example.test")
	stranger := newUser(t, app, "stranger@example.test")
	mux := recordAPI(t, app)

	quiet := conversation(t, app, owner.Id, "5216641111111", "200000000000002")
	quiet.Set("last_message_at", "2026-10-28T09:00:00.000Z")
	loud := conversation(t, app, owner.Id, "5216642222222", "200000000000002")
	loud.Set("last_message_at", "2026-10-29T18:30:05.250Z")
	middle := conversation(t, app, owner.Id, "5216643333333", "200000000000002")
	middle.Set("last_message_at", "2026-10-29T18:30:05.025Z")
	for _, record := range []*core.Record{quiet, loud, middle} {
		if err := app.Save(record); err != nil {
			t.Fatalf("Save conversation: %v", err)
		}
	}

	conversation(t, app, stranger.Id, "5216649999999", "300000000000003")

	items := listed(t, mux, tokenFor(t, owner), migrations.ConversationsCollection, "", "-last_message_at")

	if len(items) != 3 {
		t.Fatalf("listed %d conversations, want the owner's 3: %v", len(items), items)
	}

	want := []string{"5216642222222", "5216643333333", "5216641111111"}
	for i, phone := range want {
		if items[i]["customer_phone"] != phone {
			t.Errorf("conversation %d is %v, want %s", i, items[i]["customer_phone"], phone)
		}
	}
}

// The rule, not the filter, is what scopes the list — so a request that asks
// for someone else's conversations still only gets its own.
func TestAPersonCannotReadAnotherPersonsConversations(t *testing.T) {
	app := newApp(t)
	owner := newUser(t, app, "owner@example.test")
	stranger := newUser(t, app, "stranger@example.test")
	mux := recordAPI(t, app)

	conv := conversation(t, app, owner.Id, "5216641234567", "200000000000002")
	message(t, app, conv, "inbound", "Can I move my appointment?", "2026-10-29T12:00:00.000Z")

	strangerToken := tokenFor(t, stranger)

	// Asking for them by owner id.
	byOwner := listed(t, mux, strangerToken, migrations.ConversationsCollection,
		`user_id = "`+owner.Id+`"`, "-last_message_at")
	if len(byOwner) != 0 {
		t.Errorf("a stranger filtering by the owner's id read %d conversations: %v", len(byOwner), byOwner)
	}

	// Asking for the thread by the conversation id.
	byThread := listed(t, mux, strangerToken, migrations.WhatsAppMessagesCollection,
		`conversation_id = "`+conv.Id+`"`, "sent_at")
	if len(byThread) != 0 {
		t.Errorf("a stranger read %d messages from another person's thread: %v", len(byThread), byThread)
	}

	// Asking for the conversation itself, by id.
	req := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+migrations.ConversationsCollection+"/records/"+conv.Id, nil)
	req.Header.Set("Authorization", strangerToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("viewing another person's conversation: status = %d, want 404 (body: %s)", rec.Code, rec.Body)
	}

	// And an unauthenticated visitor sees nothing either.
	anonymous := httptest.NewRequest(http.MethodGet,
		"/api/collections/"+migrations.ConversationsCollection+"/records", nil)
	anonymousRec := httptest.NewRecorder()
	mux.ServeHTTP(anonymousRec, anonymous)
	if anonymousRec.Code == http.StatusOK {
		var page struct {
			Items []map[string]any `json:"items"`
		}
		_ = json.Unmarshal(anonymousRec.Body.Bytes(), &page)
		if len(page.Items) != 0 {
			t.Errorf("an anonymous visitor read %d conversations", len(page.Items))
		}
	}
}

// A thread reads oldest first, which is how a transcript is read and the order
// the AI is given its history in. sent_at is text too, so the same
// string-sorting rule applies, including across the millisecond.
func TestAThreadReadsOldestFirst(t *testing.T) {
	app := newApp(t)
	owner := newUser(t, app, "owner@example.test")
	mux := recordAPI(t, app)

	conv := conversation(t, app, owner.Id, "5216641234567", "200000000000002")
	other := conversation(t, app, owner.Id, "5216649999999", "200000000000002")

	message(t, app, conv, "outbound", "third", "2026-10-29T12:00:01.100Z")
	message(t, app, conv, "inbound", "first", "2026-10-29T12:00:00.090Z")
	message(t, app, conv, "outbound", "second", "2026-10-29T12:00:01.000Z")
	message(t, app, other, "inbound", "a different customer entirely", "2026-10-29T12:00:00.500Z")

	items := listed(t, mux, tokenFor(t, owner), migrations.WhatsAppMessagesCollection,
		`conversation_id = "`+conv.Id+`"`, "sent_at")

	if len(items) != 3 {
		t.Fatalf("read %d messages, want this conversation's 3: %v", len(items), items)
	}
	for i, want := range []string{"first", "second", "third"} {
		if items[i]["message_content"] != want {
			t.Errorf("message %d is %v, want %q", i, items[i]["message_content"], want)
		}
	}
	if items[0]["direction"] != "inbound" || items[1]["direction"] != "outbound" {
		t.Errorf("directions = %v / %v, want the stored inbound and outbound", items[0]["direction"], items[1]["direction"])
	}
}

// The browser cannot write a conversation or a message, which is why sending
// is a server route at all. If this ever stops being true, the send handler
// stops being the only way a message reaches the thread.
func TestTheBrowserCannotWriteAThreadItself(t *testing.T) {
	app := newApp(t)
	owner := newUser(t, app, "owner@example.test")
	mux := recordAPI(t, app)

	conv := conversation(t, app, owner.Id, "5216641234567", "200000000000002")
	token := tokenFor(t, owner)

	for _, write := range []struct {
		name, method, path, body string
	}{
		{
			"create a message", http.MethodPost,
			"/api/collections/" + migrations.WhatsAppMessagesCollection + "/records",
			`{"user_id":"` + owner.Id + `","to_number":"5216641234567","message_content":"I never sent this",` +
				`"direction":"outbound","conversation_id":"` + conv.Id + `","sent_at":"2026-10-29T13:00:00.000Z"}`,
		},
		{
			"create a conversation", http.MethodPost,
			"/api/collections/" + migrations.ConversationsCollection + "/records",
			`{"user_id":"` + owner.Id + `","customer_phone":"5216640000000","status":"active"}`,
		},
		{
			"edit my own conversation", http.MethodPatch,
			"/api/collections/" + migrations.ConversationsCollection + "/records/" + conv.Id,
			`{"customer_phone":"5216640000001"}`,
		},
	} {
		t.Run(write.name, func(t *testing.T) {
			req := httptest.NewRequest(write.method, write.path, jsonBody(write.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want the write refused (body: %s)", rec.Code, rec.Body)
			}
		})
	}
}

// jsonBody is a request body reader, or nil for no body at all.
func jsonBody(body string) io.Reader {
	if body == "" {
		return nil
	}

	return strings.NewReader(body)
}
