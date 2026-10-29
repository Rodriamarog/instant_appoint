package webhook_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/routersig"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// The fixtures of lib/__tests__/facebook-webhook.test.ts, kept identical so a
// case here and the case it was ported from describe the same delivery.
const (
	secret               = "test-router-secret"
	phoneNumberID        = "555000111"
	unknownPhoneNumberID = "1078215982040649"
)

var frozen = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// --- the in-memory store -------------------------------------------------

// storedMessage is what the fake wrote, including the user id the handler
// passed, which no field of webhook.StoredMessage carries.
type storedMessage struct {
	webhook.StoredMessage
	userID string
}

type conversation struct {
	id            string
	userID        string
	customerPhone string
}

// fakeStore is the in-memory stand-in for PocketBase. Everything the handler
// is asserted on happens through it, so the whole suite runs with no database
// and no network.
type fakeStore struct {
	mu            sync.Mutex
	accounts      map[string]webhook.Account
	messages      []storedMessage
	conversations []conversation

	// Hooks for the failure cases. Each runs before the real behaviour and a
	// non-nil error is returned instead of it.
	failSave          error
	failHasMessage    error
	onUpsertConversat func()
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		accounts: map[string]webhook.Account{
			phoneNumberID: {UserID: "user-1", AccessToken: "token-1"},
		},
	}
}

func (s *fakeStore) FindAccount(_ context.Context, id string) (*webhook.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	account, ok := s.accounts[id]
	if !ok {
		return nil, nil
	}

	return &account, nil
}

func (s *fakeStore) HasMessage(_ context.Context, messageID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failHasMessage != nil {
		return false, s.failHasMessage
	}
	for _, m := range s.messages {
		if m.MessageID == messageID {
			return true, nil
		}
	}

	return false, nil
}

func (s *fakeStore) UpsertConversation(_ context.Context, userID, customerPhone, _, _ string) (string, error) {
	if s.onUpsertConversat != nil {
		s.onUpsertConversat()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, c := range s.conversations {
		if c.userID == userID && c.customerPhone == customerPhone {
			return c.id, nil
		}
	}

	created := conversation{
		id:            fmt.Sprintf("conv-%d", len(s.conversations)+1),
		userID:        userID,
		customerPhone: customerPhone,
	}
	s.conversations = append(s.conversations, created)

	return created.id, nil
}

func (s *fakeStore) SaveMessage(_ context.Context, userID string, m webhook.StoredMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failSave != nil {
		return s.failSave
	}
	s.messages = append(s.messages, storedMessage{StoredMessage: m, userID: userID})

	return nil
}

func (s *fakeStore) ConversationHistory(_ context.Context, conversationID string) ([]webhook.StoredMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []webhook.StoredMessage
	for _, m := range s.messages {
		if m.ConversationID == conversationID {
			out = append(out, m.StoredMessage)
		}
	}

	return out, nil
}

// ids returns the message ids the store holds, in write order.
func (s *fakeStore) ids() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := []string{}
	for _, m := range s.messages {
		out = append(out, m.MessageID)
	}

	return out
}

// inboundIDs returns only the ids of inbound messages, in write order.
func (s *fakeStore) inboundIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := []string{}
	for _, m := range s.messages {
		if m.Direction == webhook.Inbound {
			out = append(out, m.MessageID)
		}
	}

	return out
}

func (s *fakeStore) count(messageID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, m := range s.messages {
		if m.MessageID == messageID {
			n++
		}
	}

	return n
}

// --- the harness ---------------------------------------------------------

// send is one call the handler made to WhatsApp.
type send struct {
	phoneNumberID string
	to            string
	text          string
}

type harness struct {
	handler http.Handler
	store   *fakeStore

	mu      sync.Mutex
	sent    []send
	history [][]webhook.ConversationMessage
}

type options struct {
	secret        string
	store         webhook.Store
	generateReply func(context.Context, []webhook.ConversationMessage, string) (string, error)
	sendMessage   func(context.Context, string, string, string, string) (string, error)
}

func setup(opts options) *harness {
	store := newFakeStore()
	h := &harness{store: store}

	deps := webhook.Deps{
		Secret: secret,
		Now:    func() time.Time { return frozen },
		Store:  store,
		GenerateReply: func(_ context.Context, history []webhook.ConversationMessage, text string) (string, error) {
			h.mu.Lock()
			h.history = append(h.history, history)
			h.mu.Unlock()

			return "reply to: " + text, nil
		},
		SendMessage: func(_ context.Context, pid, _, to, text string) (string, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.sent = append(h.sent, send{phoneNumberID: pid, to: to, text: text})

			return fmt.Sprintf("wamid.reply-%d", len(h.sent)), nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	if opts.secret != "" {
		deps.Secret = opts.secret
	}
	if opts.secret == "-" {
		deps.Secret = ""
	}
	if opts.store != nil {
		deps.Store = opts.store
	}
	if opts.generateReply != nil {
		deps.GenerateReply = opts.generateReply
	}
	if opts.sendMessage != nil {
		deps.SendMessage = opts.sendMessage
	}

	h.handler = webhook.New(deps)

	return h
}

func (h *harness) sends() []send {
	h.mu.Lock()
	defer h.mu.Unlock()

	return append([]send{}, h.sent...)
}

func (h *harness) sentTo() []string {
	out := []string{}
	for _, s := range h.sends() {
		out = append(out, s.to)
	}

	return out
}

// post signs payload the way the router does and runs it through the handler.
func (h *harness) post(t *testing.T, payload any, opts ...func(*delivery)) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal the payload: %v", err)
	}

	return h.postRaw(t, string(body), opts...)
}

type delivery struct {
	secret      string
	timestamp   string
	unsigned    bool
	contentType string
}

func (h *harness) postRaw(t *testing.T, body string, opts ...func(*delivery)) *httptest.ResponseRecorder {
	t.Helper()

	d := delivery{secret: secret, timestamp: strconv.FormatInt(frozen.Unix(), 10), contentType: "application/json"}
	for _, apply := range opts {
		apply(&d)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/webhook", strings.NewReader(body))
	request.Header.Set("content-type", d.contentType)
	if !d.unsigned {
		mac := hmac.New(sha256.New, []byte(d.secret))
		mac.Write([]byte(d.timestamp + "." + body))
		request.Header.Set(routersig.SignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))
		request.Header.Set(routersig.TimestampHeader, d.timestamp)
		request.Header.Set(routersig.DeliveryIDHeader, "d-1")
	}

	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, request)

	return response
}

func signedWith(key string) func(*delivery) {
	return func(d *delivery) { d.secret = key }
}

func at(when time.Time) func(*delivery) {
	return func(d *delivery) { d.timestamp = strconv.FormatInt(when.Unix(), 10) }
}

func unsigned(d *delivery) { d.unsigned = true }

// --- payload builders ----------------------------------------------------

type change map[string]any

// textMessage is the inbound message shape Meta delivers.
func textMessage(id, from, body string, sentAt time.Time) map[string]any {
	return map[string]any{
		"id":        id,
		"from":      from,
		"type":      "text",
		"text":      map[string]any{"body": body},
		"timestamp": strconv.FormatInt(sentAt.Unix(), 10),
	}
}

// messages is a "messages" change carrying the given inbound messages from the
// default connected number.
func messages(items ...map[string]any) change {
	return messagesFrom(phoneNumberID, items...)
}

func messagesFrom(pid string, items ...map[string]any) change {
	return change{
		"field": "messages",
		"value": map[string]any{
			"messaging_product": "whatsapp",
			"metadata":          map[string]any{"phone_number_id": pid},
			"messages":          items,
		},
	}
}

// payload wraps changes into entries: one slice of changes per entry.
func payload(entries ...[]change) map[string]any {
	built := []any{}
	for i, changes := range entries {
		raw := []any{}
		for _, c := range changes {
			raw = append(raw, map[string]any(c))
		}
		built = append(built, map[string]any{"id": fmt.Sprintf("waba-%d", i), "changes": raw})
	}

	return map[string]any{"object": "whatsapp_business_account", "entry": built}
}

// --- assertions ----------------------------------------------------------

func assertStatus(t *testing.T, got *httptest.ResponseRecorder, want int) {
	t.Helper()

	if got.Code != want {
		t.Errorf("status = %d, want %d (body %s)", got.Code, want, strings.TrimSpace(got.Body.String()))
	}
}

func assertEqual[T any](t *testing.T, label string, got, want T) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", label, got, want)
	}
}

// --- 1-6: the signature gate (describe 'webhook POST from the router') ---

// Legacy case 1.
func TestRejectsADeliveryWithNoSignature(t *testing.T) {
	h := setup(options{})

	assertStatus(t, h.postRaw(t, `{"entry":[]}`, unsigned), http.StatusUnauthorized)
}

// Legacy case 2.
func TestRejectsADeliverySignedWithADifferentSecret(t *testing.T) {
	h := setup(options{})

	assertStatus(t, h.post(t, map[string]any{"entry": []any{}}, signedWith("someone-else")), http.StatusUnauthorized)
}

// Legacy case 3.
func TestRejectsADeliveryMoreThanFiveMinutesOld(t *testing.T) {
	h := setup(options{})

	assertStatus(t, h.post(t, map[string]any{"entry": []any{}}, at(frozen.Add(-6*time.Minute))), http.StatusUnauthorized)
}

// Legacy case 4.
func TestRejectsADeliveryMoreThanFiveMinutesInTheFuture(t *testing.T) {
	h := setup(options{})

	assertStatus(t, h.post(t, map[string]any{"entry": []any{}}, at(frozen.Add(6*time.Minute))), http.StatusUnauthorized)
}

// Legacy case 5. The delivery is signed with the same empty secret the handler
// holds, so the digest matches: only the explicit guard rejects it.
func TestRejectsEverythingWhenNoSecretIsConfigured(t *testing.T) {
	h := setup(options{secret: "-"})

	assertStatus(t, h.post(t, map[string]any{"entry": []any{}}, signedWith("")), http.StatusUnauthorized)
}

// Legacy case 6.
func TestAcceptsADeliveryFromFourMinutesAgo(t *testing.T) {
	h := setup(options{})

	assertStatus(t, h.post(t, map[string]any{"entry": []any{}}, at(frozen.Add(-4*time.Minute))), http.StatusOK)
}

// --- 7-14: processing a signed payload -----------------------------------

// Legacy case 7.
func TestStoresAnInboundMessageAndSendsAndStoresAnAIReply(t *testing.T) {
	h := setup(options{})

	response := h.post(t, payload([]change{messages(textMessage("wamid.1", "5215511111111", "hola", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "sent", h.sends(), []send{{phoneNumberID: phoneNumberID, to: "5215511111111", text: "reply to: hola"}})

	got := [][3]string{}
	for _, m := range h.store.messages {
		got = append(got, [3]string{m.MessageID, string(m.Direction), m.Content})
	}
	assertEqual(t, "stored", got, [][3]string{
		{"wamid.1", "inbound", "hola"},
		{"wamid.reply-1", "outbound", "reply to: hola"},
	})
}

// Legacy case 8. Order is load-bearing: the reply to the first message must be
// generated from a history that does not yet contain the second.
func TestProcessesEveryMessageInEveryChangeOfEveryEntry(t *testing.T) {
	h := setup(options{})

	response := h.post(t, payload(
		[]change{
			messages(
				textMessage("wamid.a1", "521000000001", "uno", frozen),
				textMessage("wamid.a2", "521000000002", "dos", frozen),
			),
			messages(textMessage("wamid.b1", "521000000003", "tres", frozen)),
		},
		[]change{messages(textMessage("wamid.c1", "521000000004", "cuatro", frozen))},
	))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "inbound ids", h.store.inboundIDs(), []string{"wamid.a1", "wamid.a2", "wamid.b1", "wamid.c1"})
	assertEqual(t, "recipients", h.sentTo(), []string{"521000000001", "521000000002", "521000000003", "521000000004"})
}

// Legacy case 9. An echo is the business's own message, captured so the
// conversation reads correctly in the dashboard. Replying to one would mean
// the bot answering the business.
func TestStoresOutboundEchoesWithoutReplying(t *testing.T) {
	h := setup(options{})

	echo := change{
		"field": "smb_message_echoes",
		"value": map[string]any{
			"metadata": map[string]any{"phone_number_id": phoneNumberID},
			"message_echoes": []any{map[string]any{
				"id":        "wamid.echo1",
				"to":        "521000000009",
				"type":      "text",
				"text":      map[string]any{"body": "ya voy"},
				"timestamp": strconv.FormatInt(frozen.Unix(), 10),
			}},
		},
	}

	assertStatus(t, h.post(t, payload([]change{echo})), http.StatusOK)

	got := [][3]string{}
	for _, m := range h.store.messages {
		got = append(got, [3]string{m.MessageID, string(m.Direction), m.CustomerPhone})
	}
	assertEqual(t, "stored", got, [][3]string{{"wamid.echo1", "outbound", "521000000009"}})
	assertEqual(t, "sent", h.sends(), []send{})
}

// Legacy case 10. The router delivers at least once, so this is the ordinary
// case, not an edge case.
func TestIgnoresARedeliveredMessage(t *testing.T) {
	h := setup(options{})
	delivery := payload([]change{messages(textMessage("wamid.dup", "5215511111111", "hola", frozen))})

	assertStatus(t, h.post(t, delivery), http.StatusOK)
	assertStatus(t, h.post(t, delivery), http.StatusOK)

	if got := h.store.count("wamid.dup"); got != 1 {
		t.Errorf("stored wamid.dup %d times, want 1", got)
	}
	if got := len(h.sends()); got != 1 {
		t.Errorf("sent %d replies, want 1", got)
	}
}

// Legacy case 11.
func TestHandlesTheSameMessageIDTwiceWithinOnePayloadOnlyOnce(t *testing.T) {
	h := setup(options{})
	message := textMessage("wamid.twice", "5215511111111", "hola", frozen)

	h.post(t, payload([]change{messages(message)}, []change{messages(message)}))

	if got := h.store.count("wamid.twice"); got != 1 {
		t.Errorf("stored wamid.twice %d times, want 1", got)
	}
	if got := len(h.sends()); got != 1 {
		t.Errorf("sent %d replies, want 1", got)
	}
}

// Legacy case 12. A backlog delivered after an outage is history, not a
// conversation: answering it means replying to questions hours late.
func TestStoresAMessageOlderThanTheCutoffWithoutReplying(t *testing.T) {
	h := setup(options{})

	response := h.post(t, payload([]change{
		messages(textMessage("wamid.late", "5215511111111", "hola?", frozen.Add(-16*time.Minute))),
	}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "stored", h.store.ids(), []string{"wamid.late"})
	assertEqual(t, "sent", h.sends(), []send{})
}

// Legacy case 13.
func TestStillRepliesToAMessageFourteenMinutesOld(t *testing.T) {
	h := setup(options{})

	h.post(t, payload([]change{
		messages(textMessage("wamid.recent", "5215511111111", "hola", frozen.Add(-14*time.Minute))),
	}))

	if got := len(h.sends()); got != 1 {
		t.Errorf("sent %d replies, want 1", got)
	}
}

// Legacy case 14. No timestamp means the age cannot be established, and the
// handler refuses to guess.
func TestDoesNotReplyToAMessageWithNoTimestamp(t *testing.T) {
	h := setup(options{})

	message := textMessage("wamid.nots", "5215511111111", "hola", frozen)
	delete(message, "timestamp")
	h.post(t, payload([]change{messages(message)}))

	assertEqual(t, "stored", h.store.ids(), []string{"wamid.nots"})
	assertEqual(t, "sent", h.sends(), []send{})
}

// --- 15: numbers without an account --------------------------------------

// Legacy case 15. The router forwards every number it serves, most of which
// belong to other apps behind it.
func TestIgnoresNumbersWithoutAnAccountAndProcessesTheRest(t *testing.T) {
	h := setup(options{})

	response := h.post(t, payload(
		[]change{messagesFrom(unknownPhoneNumberID, textMessage("wamid.unknown", "521000000001", "cita", frozen))},
		[]change{messages(textMessage("wamid.ia", "521000000002", "hola", frozen))},
	))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "inbound ids", h.store.inboundIDs(), []string{"wamid.ia"})

	numbers := []string{}
	for _, s := range h.sends() {
		numbers = append(numbers, s.phoneNumberID)
	}
	assertEqual(t, "sending numbers", numbers, []string{phoneNumberID})
}

// --- 16-17: failures -----------------------------------------------------

// Legacy case 16. A non-2xx is the only way to ask the router to deliver
// again; swallowing a storage failure loses the message silently.
func TestReturnsFiveHundredWhenStoringFails(t *testing.T) {
	store := newFakeStore()
	store.failSave = errors.New("PocketBase down")
	h := setup(options{store: store})

	response := h.post(t, payload([]change{messages(textMessage("wamid.f", "521000000001", "hola", frozen))}))

	assertStatus(t, response, http.StatusInternalServerError)
}

// Legacy case 17. The message is already stored, so a retry would store
// nothing new and only re-run the AI step; a 200 keeps the router from
// retrying a delivery that succeeded at the part that matters.
func TestStillReturnsTwoHundredWhenTheAIReplyFails(t *testing.T) {
	h := setup(options{
		generateReply: func(context.Context, []webhook.ConversationMessage, string) (string, error) {
			return "", errors.New("Gemini down")
		},
		sendMessage: func(context.Context, string, string, string, string) (string, error) {
			return "never", nil
		},
	})

	response := h.post(t, payload([]change{messages(textMessage("wamid.g", "521000000001", "hola", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "stored", h.store.ids(), []string{"wamid.g"})
}

// --- behaviours the vitest suite did not cover ---------------------------

func TestRejectsASignedBodyThatIsNotJSON(t *testing.T) {
	h := setup(options{})

	assertStatus(t, h.postRaw(t, "not json at all"), http.StatusBadRequest)
}

// The handler answers in PocketBase's error shape, so a failure from this
// route and a failure from any other route parse the same way.
func TestErrorsUsePocketBasesShape(t *testing.T) {
	h := setup(options{})

	var body struct {
		Message string          `json:"message"`
		Status  int             `json:"status"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(h.postRaw(t, `{}`, unsigned).Body.Bytes(), &body); err != nil {
		t.Fatalf("the error body is not JSON: %v", err)
	}
	if body.Status != http.StatusUnauthorized || body.Message == "" || string(body.Data) != "{}" {
		t.Errorf("error body = %+v, want a non-empty message, status 401 and an empty data object", body)
	}
}

func TestSkipsItemsTheHandlerCannotStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		item map[string]any
	}{
		{"an image", map[string]any{"id": "wamid.x", "from": "521000000001", "type": "image", "image": map[string]any{"id": "media-1"}}},
		{"a text message with no body", map[string]any{"id": "wamid.x", "from": "521000000001", "type": "text", "text": map[string]any{}}},
		{"a text body that is not a string", map[string]any{"id": "wamid.x", "from": "521000000001", "type": "text", "text": map[string]any{"body": 42}}},
		{"no message id", map[string]any{"from": "521000000001", "type": "text", "text": map[string]any{"body": "hola"}}},
		{"no sender", map[string]any{"id": "wamid.x", "type": "text", "text": map[string]any{"body": "hola"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := setup(options{})

			assertStatus(t, h.post(t, payload([]change{messages(tc.item)})), http.StatusOK)
			assertEqual(t, "stored", h.store.ids(), []string{})
		})
	}
}

// Meta sends delivery receipts, template approvals and more down the same
// webhook. Only the two message fields are ours.
func TestSkipsChangesItDoesNotUnderstand(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    change
	}{
		{
			"a statuses change",
			change{"field": "statuses", "value": map[string]any{
				"metadata": map[string]any{"phone_number_id": phoneNumberID},
				"statuses": []any{map[string]any{"id": "wamid.s", "status": "delivered"}},
			}},
		},
		{
			"a change with no phone_number_id",
			change{"field": "messages", "value": map[string]any{
				"messages": []any{textMessage("wamid.x", "521000000001", "hola", frozen)},
			}},
		},
		{
			"a change whose phone_number_id is empty",
			change{"field": "messages", "value": map[string]any{
				"metadata": map[string]any{"phone_number_id": ""},
				"messages": []any{textMessage("wamid.x", "521000000001", "hola", frozen)},
			}},
		},
		{
			"a change whose messages are not an array",
			change{"field": "messages", "value": map[string]any{
				"metadata": map[string]any{"phone_number_id": phoneNumberID},
				"messages": "nope",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := setup(options{})

			assertStatus(t, h.post(t, payload([]change{tc.c})), http.StatusOK)
			assertEqual(t, "stored", h.store.ids(), []string{})
		})
	}
}

// The reply is generated from the conversation so far, with the message that
// triggered it passed separately rather than appearing twice.
func TestTheAIHistoryExcludesTheTriggeringMessage(t *testing.T) {
	h := setup(options{})

	h.post(t, payload([]change{messages(textMessage("wamid.1", "5215511111111", "hola", frozen))}))
	h.post(t, payload([]change{messages(textMessage("wamid.2", "5215511111111", "el martes", frozen))}))

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.history) != 2 {
		t.Fatalf("generateReply called %d times, want 2", len(h.history))
	}
	if len(h.history[0]) != 0 {
		t.Errorf("the first reply saw a history of %#v, want nothing before it", h.history[0])
	}
	assertEqual(t, "second history", h.history[1], []webhook.ConversationMessage{
		{Role: webhook.RoleUser, Content: "hola"},
		{Role: webhook.RoleModel, Content: "reply to: hola"},
	})
}

// Both calls read the same frozen clock, so this asserts the format rather
// than the instant: the stored timestamp is the receive time in the one shape
// the rest of the app sorts and filters on.
func TestStoresTheReceiveTimeInTheAppsTimestampFormat(t *testing.T) {
	h := setup(options{})

	h.post(t, payload([]change{
		messages(textMessage("wamid.1", "5215511111111", "hola", frozen.Add(-time.Minute))),
	}))

	for _, m := range h.store.messages {
		if m.SentAt != "2026-10-13T12:00:00.000Z" {
			t.Errorf("%s stored sent_at %q, want the receive time 2026-10-13T12:00:00.000Z", m.MessageID, m.SentAt)
		}
	}
}

// Two deliveries of one message can be in flight at once, and both would pass
// a store lookup made before either has written. The guard is what stops the
// customer getting two replies.
func TestConcurrentDeliveriesOfOneMessageProduceOneReply(t *testing.T) {
	store := newFakeStore()
	reached, release := make(chan struct{}), make(chan struct{})
	// A one-shot token rather than sync.Once, which would block the second
	// caller until the first returns and so hide a missing guard behind a
	// deadlock instead of a failed assertion.
	first := make(chan struct{}, 1)
	first <- struct{}{}
	// Park the first delivery mid-processing: it has passed the store's
	// duplicate check and has not yet written anything, which is the window a
	// store lookup alone cannot close.
	store.onUpsertConversat = func() {
		select {
		case <-first:
			close(reached)
			<-release
		default:
		}
	}
	h := setup(options{store: store})
	delivery := payload([]change{messages(textMessage("wamid.race", "5215511111111", "hola", frozen))})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); h.post(t, delivery) }()
	go func() {
		defer wg.Done()
		<-reached
		h.post(t, delivery)
		close(release)
	}()
	wg.Wait()

	if got := store.count("wamid.race"); got != 1 {
		t.Errorf("stored wamid.race %d times, want 1", got)
	}
	if got := len(h.sends()); got != 1 {
		t.Errorf("sent %d replies, want 1", got)
	}
}
