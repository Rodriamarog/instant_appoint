package pbstore_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/Rodriamarog/instant_appoint/internal/gemini"
	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
	"github.com/Rodriamarog/instant_appoint/internal/routersig"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// The unit tests prove each piece against fakes. This proves they were
// assembled correctly: a signed delivery arrives over real HTTP, through the
// real route registration, into a real SQLite database, and comes back out as
// a real HTTP call to WhatsApp.
//
// Only the two outside services are stubbed, and only by pointing their
// clients' BaseURL at an httptest server — the same clients, the same
// requests. Nothing here reaches the network.

const routerSecret = "integration-router-secret"

// stack is the whole inbound path, assembled the way cmd/server assembles it.
type stack struct {
	app core.App
	url string

	// userID is the clinic behind the one connected number, which is what a
	// test changing the AI mode has to name.
	userID string

	meta  *recorder
	ai    *recorder
	reply string
}

// recorder counts calls to a stubbed service and keeps the last body it saw.
type recorder struct {
	calls int
	paths []string
	last  map[string]any
}

func (r *recorder) record(req *http.Request) {
	r.calls++
	r.paths = append(r.paths, req.URL.Path)

	raw, _ := io.ReadAll(req.Body)
	r.last = map[string]any{}
	_ = json.Unmarshal(raw, &r.last)
}

func newStack(t *testing.T) *stack {
	t.Helper()

	s := &stack{app: newApp(t), meta: &recorder{}, ai: &recorder{}, reply: "¿Te viene bien el martes a las 10?"}
	s.userID = connectAccount(t, s.app, "555000111", nil)
	// On autopilot, which is the only mode that sends anything and so the
	// only one in which the rest of this file has something to assert. The
	// clinic that has not switched the assistant on is the last test here.
	setAIMode(t, s.app, s.userID, "autopilot")

	// Gemini, answering with one candidate.
	geminiStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.ai.record(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{map[string]any{
				"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": s.reply}}},
				"finishReason": "STOP",
			}},
		})
	}))
	t.Cleanup(geminiStub.Close)

	// Meta's Graph API, accepting a send and returning a wamid.
	metaStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.meta.record(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": []any{map[string]any{"id": fmt.Sprintf("wamid.reply-%d", s.meta.calls)}},
		})
	}))
	t.Cleanup(metaStub.Close)

	ai := gemini.New("stub-key")
	ai.BaseURL = geminiStub.URL

	whatsApp := meta.New("app-id", "app-secret")
	whatsApp.BaseURL = metaStub.URL

	handler := webhook.New(webhook.Deps{
		Secret: routerSecret,
		Now:    time.Now,
		Store:  pbstore.NewWebhookStore(s.app),
		GenerateReply: func(ctx context.Context, history []webhook.ConversationMessage, text string) (string, error) {
			turns := make([]gemini.Message, 0, len(history))
			for _, turn := range history {
				turns = append(turns, gemini.Message{Role: turn.Role, Content: turn.Content})
			}

			return ai.GenerateReply(ctx, turns, text)
		},
		SendMessage: whatsApp.SendText,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	r := router.NewRouter(func(w http.ResponseWriter, req *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		event := new(core.RequestEvent)
		event.Response, event.Request = w, req

		return event, nil
	})
	pbstore.RegisterWebhook(r, handler)

	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	s.url = server.URL

	return s
}

// deliver posts a payload the way the Neurocrow router does.
func (s *stack) deliver(t *testing.T, path, body, secret string, when time.Time) int {
	t.Helper()

	timestamp := strconv.FormatInt(when.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + body))

	request, err := http.NewRequest(http.MethodPost, s.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(routersig.SignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set(routersig.TimestampHeader, timestamp)
	request.Header.Set(routersig.DeliveryIDHeader, "d-1")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post the delivery: %v", err)
	}
	defer response.Body.Close() //nolint:errcheck // the status is all this needs

	return response.StatusCode
}

// deliveryBody is one inbound text message from a customer, as Meta sends it.
func deliveryBody(messageID, from, text string, at time.Time) string {
	payload, _ := json.Marshal(map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{map[string]any{
			"id": "waba-1",
			"changes": []any{map[string]any{
				"field": "messages",
				"value": map[string]any{
					"messaging_product": "whatsapp",
					"metadata":          map[string]any{"phone_number_id": "555000111"},
					"messages": []any{map[string]any{
						"id": messageID, "from": from, "type": "text",
						"text":      map[string]any{"body": text},
						"timestamp": strconv.FormatInt(at.Unix(), 10),
					}},
				},
			}},
		}},
	})

	return string(payload)
}

func (s *stack) count(t *testing.T, collection string) int {
	t.Helper()

	n, err := s.app.CountRecords(collection)
	if err != nil {
		t.Fatalf("CountRecords(%q): %v", collection, err)
	}

	return int(n)
}

func TestASignedDeliveryIsStoredAndAnswered(t *testing.T) {
	s := newStack(t)
	now := time.Now()

	if status := s.deliver(t, "/api/webhook", deliveryBody("wamid.1", "5215511111111", "hola", now), routerSecret, now); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if got := s.count(t, "conversations"); got != 1 {
		t.Errorf("%d conversations, want 1", got)
	}
	if got := s.count(t, "whatsapp_messages"); got != 2 {
		t.Errorf("%d messages, want the customer's and the reply", got)
	}

	// The reply went to Meta, addressed to the customer, carrying what the
	// model wrote.
	if s.meta.calls != 1 {
		t.Fatalf("WhatsApp was called %d times, want once", s.meta.calls)
	}
	if want := "/v21.0/555000111/messages"; s.meta.paths[0] != want {
		t.Errorf("sent to %s, want %s", s.meta.paths[0], want)
	}
	if to := s.meta.last["to"]; to != "5215511111111" {
		t.Errorf("sent to %v, want the customer", to)
	}
	body, _ := s.meta.last["text"].(map[string]any)
	if body == nil || body["body"] != s.reply {
		t.Errorf("sent %v, want the model's reply %q", s.meta.last["text"], s.reply)
	}

	// And it was stored, so the dashboard and the next turn's history have it.
	stored, err := s.app.FindFirstRecordByFilter("whatsapp_messages", "direction = 'outbound'")
	if err != nil {
		t.Fatalf("read the stored reply: %v", err)
	}
	if stored.GetString("message_content") != s.reply {
		t.Errorf("stored reply = %q, want %q", stored.GetString("message_content"), s.reply)
	}
	if stored.GetString("message_id") != "wamid.reply-1" {
		t.Errorf("stored message_id = %q, want the wamid Meta returned", stored.GetString("message_id"))
	}
}

func TestARedeliveryChangesNothing(t *testing.T) {
	s := newStack(t)
	now := time.Now()
	body := deliveryBody("wamid.dup", "5215511111111", "hola", now)

	for i := 0; i < 3; i++ {
		if status := s.deliver(t, "/api/webhook", body, routerSecret, now); status != http.StatusOK {
			t.Fatalf("delivery %d: status = %d, want 200", i+1, status)
		}
	}

	if got := s.count(t, "whatsapp_messages"); got != 2 {
		t.Errorf("%d messages after three deliveries, want 2", got)
	}
	if got := s.count(t, "conversations"); got != 1 {
		t.Errorf("%d conversations after three deliveries, want 1", got)
	}
	if s.meta.calls != 1 {
		t.Errorf("the customer was messaged %d times, want once", s.meta.calls)
	}
}

func TestADeliveryThatCannotBeTrustedIsRejected(t *testing.T) {
	now := time.Now()

	for _, tc := range []struct {
		name   string
		secret string
		when   time.Time
	}{
		{"signed with the wrong secret", "someone-elses-secret", now},
		{"signed with no secret at all", "", now},
		{"timestamped outside the window", routerSecret, now.Add(-6 * time.Minute)},
		{"timestamped too far ahead", routerSecret, now.Add(6 * time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStack(t)
			body := deliveryBody("wamid.bad", "5215511111111", "hola", now)

			if status := s.deliver(t, "/api/webhook", body, tc.secret, tc.when); status != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", status)
			}
			if got := s.count(t, "whatsapp_messages"); got != 0 {
				t.Errorf("%d messages were stored from a delivery that was rejected", got)
			}
			if s.meta.calls != 0 {
				t.Errorf("a rejected delivery still messaged the customer %d times", s.meta.calls)
			}
		})
	}
}

// A delivery with no router headers at all — what an attacker who found the
// URL would send.
func TestAnUnsignedDeliveryIsRejected(t *testing.T) {
	s := newStack(t)
	now := time.Now()

	response, err := http.Post(s.url+"/api/webhook", "application/json",
		strings.NewReader(deliveryBody("wamid.bad", "5215511111111", "hola", now)))
	if err != nil {
		t.Fatalf("post the delivery: %v", err)
	}
	defer response.Body.Close() //nolint:errcheck // the status is all this needs

	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", response.StatusCode)
	}
	if got := s.count(t, "whatsapp_messages"); got != 0 {
		t.Errorf("%d messages were stored from an unsigned delivery", got)
	}
}

// The second path the old app exposed. The router has not been confirmed to
// have stopped using it, and a delivery to a path that 404s is a lost message.
func TestTheLegacyPathWorksToo(t *testing.T) {
	s := newStack(t)
	now := time.Now()

	if status := s.deliver(t, "/webhook", deliveryBody("wamid.legacy", "5215511111111", "hola", now), routerSecret, now); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := s.count(t, "whatsapp_messages"); got != 2 {
		t.Errorf("%d messages, want the customer's and the reply", got)
	}
}

// The second turn of a conversation: the model is given what was said before,
// oldest first, with the message it is answering passed separately.
func TestTheSecondTurnCarriesTheConversationSoFar(t *testing.T) {
	s := newStack(t)
	now := time.Now()

	if status := s.deliver(t, "/api/webhook", deliveryBody("wamid.1", "5215511111111", "hola", now), routerSecret, now); status != http.StatusOK {
		t.Fatalf("the first turn: status 200 expected")
	}
	if status := s.deliver(t, "/api/webhook", deliveryBody("wamid.2", "5215511111111", "el martes", now), routerSecret, now); status != http.StatusOK {
		t.Fatalf("the second turn: status 200 expected")
	}

	contents, _ := s.ai.last["contents"].([]any)
	if len(contents) != 3 {
		t.Fatalf("the model was given %d turns, want the two stored plus the new one: %v", len(contents), s.ai.last["contents"])
	}

	for i, want := range []struct{ role, text string }{
		{"user", "hola"},
		{"model", s.reply},
		{"user", "el martes"},
	} {
		turn, _ := contents[i].(map[string]any)
		parts, _ := turn["parts"].([]any)
		part, _ := parts[0].(map[string]any)
		if turn["role"] != want.role || part["text"] != want.text {
			t.Errorf("turn %d = %v, want %s saying %q", i, turn, want.role, want.text)
		}
	}

	// Still one conversation, four messages.
	if got := s.count(t, "conversations"); got != 1 {
		t.Errorf("%d conversations, want 1", got)
	}
	if got := s.count(t, "whatsapp_messages"); got != 4 {
		t.Errorf("%d messages, want 4", got)
	}
}

// An echo of what the business typed on their own phone is stored so the
// dashboard and the model see it, and is never answered.
func TestAnEchoIsStoredAndNotAnswered(t *testing.T) {
	s := newStack(t)
	now := time.Now()

	payload, _ := json.Marshal(map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{map[string]any{
			"id": "waba-1",
			"changes": []any{map[string]any{
				"field": "smb_message_echoes",
				"value": map[string]any{
					"metadata": map[string]any{"phone_number_id": "555000111"},
					"message_echoes": []any{map[string]any{
						"id": "wamid.echo", "to": "5215511111111", "type": "text",
						"text":      map[string]any{"body": "ya voy"},
						"timestamp": strconv.FormatInt(now.Unix(), 10),
					}},
				},
			}},
		}},
	})

	if status := s.deliver(t, "/api/webhook", string(payload), routerSecret, now); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if got := s.count(t, "whatsapp_messages"); got != 1 {
		t.Errorf("%d messages, want only the echo", got)
	}
	if s.meta.calls != 0 {
		t.Errorf("the bot answered the business %d times", s.meta.calls)
	}

	stored, err := s.app.FindFirstRecordByFilter("whatsapp_messages", "message_id = {:id}", dbx.Params{"id": "wamid.echo"})
	if err != nil {
		t.Fatalf("read the echo back: %v", err)
	}
	if stored.GetString("direction") != "outbound" || stored.GetString("to_number") != "5215511111111" {
		t.Errorf("the echo was stored as %s to %s", stored.GetString("direction"), stored.GetString("to_number"))
	}
}

// Gemini falling over must not cost the message. It is already stored, and a
// non-2xx would have the router redeliver it and re-run the AI step against a
// message the database already has.
func TestAFailingAIStepStillAcknowledgesTheDelivery(t *testing.T) {
	s := newStack(t)
	now := time.Now()

	// An answer the client cannot use: blocked, which is one of the ways a
	// live call fails without the network failing.
	s.reply = ""

	if status := s.deliver(t, "/api/webhook", deliveryBody("wamid.1", "5215511111111", "hola", now), routerSecret, now); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got := s.count(t, "whatsapp_messages"); got != 1 {
		t.Errorf("%d messages, want the customer's message stored", got)
	}
	if s.meta.calls != 0 {
		t.Errorf("an empty reply was sent to the customer %d times", s.meta.calls)
	}
}

// The switch, through the whole assembled stack rather than against a fake.
// A clinic that has not turned the assistant on gets its patient's message
// stored and nothing sent — and the proof that nothing was sent is that
// neither Gemini nor Meta was called at all.
func TestADeliveryToAClinicWithTheAssistantOffIsStoredAndUnanswered(t *testing.T) {
	for _, mode := range []string{"off", "observer"} {
		s := newStack(t)
		setAIMode(t, s.app, s.userID, mode)
		now := time.Now()

		status := s.deliver(t, "/api/webhook", deliveryBody("wamid.1", "5215511111111", "hola", now), routerSecret, now)
		if status != http.StatusOK {
			t.Fatalf("in %q the status was %d, want 200", mode, status)
		}

		// Stored, because the clinic still reads its own conversations.
		if got := s.count(t, "whatsapp_messages"); got != 1 {
			t.Errorf("in %q there are %d messages, want only the customer's", mode, got)
		}
		if got := s.count(t, "conversations"); got != 1 {
			t.Errorf("in %q there are %d conversations, want 1", mode, got)
		}

		if s.ai.calls != 0 {
			t.Errorf("in %q the model was called %d times, want none", mode, s.ai.calls)
		}
		if s.meta.calls != 0 {
			t.Errorf("in %q WhatsApp was called %d times, want none", mode, s.meta.calls)
		}
	}
}

// tapDeliveryBody is a tap on a quick reply, as Meta sends it: a message of
// type "button" naming the label, and a context pointing at the reminder the
// button hung under.
func tapDeliveryBody(messageID, from, label, repliesTo string, at time.Time) string {
	payload, _ := json.Marshal(map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{map[string]any{
			"id": "waba-1",
			"changes": []any{map[string]any{
				"field": "messages",
				"value": map[string]any{
					"messaging_product": "whatsapp",
					"metadata":          map[string]any{"phone_number_id": "555000111"},
					"messages": []any{map[string]any{
						"id": messageID, "from": from, "type": "button",
						"button":    map[string]any{"payload": label, "text": label},
						"context":   map[string]any{"from": "555000111", "id": repliesTo},
						"timestamp": strconv.FormatInt(at.Unix(), 10),
					}},
				},
			}},
		}},
	})

	return string(payload)
}

// The whole of this ticket in one pass: a reminder went out about a real
// appointment, the patient taps Cancel, and the booking on the clinic's
// screen is cancelled by the patient — over real HTTP, through the real route
// registration, into a real database.
//
// It also pins the half the unit tests cannot: that nothing on the way asks
// the assistant to answer a tap.
func TestATapOnCancelCancelsTheAppointmentEndToEnd(t *testing.T) {
	s := newStack(t)

	appointmentID := saveAppointment(t, s.app, s.userID)
	saveReminder(t, s.app, s.userID, "wamid.reminder-1", appointmentID)

	body := tapDeliveryBody("wamid.tap-1", "5215511111111", "Cancelar", "wamid.reminder-1", time.Now())
	if status := s.deliver(t, "/api/webhook", body, routerSecret, time.Now()); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	record, err := s.app.FindRecordById("appointments", appointmentID)
	if err != nil {
		t.Fatalf("read the appointment back: %v", err)
	}
	if got := record.GetString("status"); got != "cancelled" {
		t.Errorf("status = %q, want cancelled", got)
	}
	if got := record.GetString("status_set_by"); got != "patient" {
		t.Errorf("status_set_by = %q, want patient", got)
	}
	if record.GetString("status_set_at") == "" {
		t.Error("status_set_at is empty; nothing says when the patient cancelled")
	}

	if s.ai.calls != 0 {
		t.Errorf("the assistant was asked to answer a tap %d times", s.ai.calls)
	}
	if s.meta.calls != 0 {
		t.Errorf("%d messages were sent in reply to a tap", s.meta.calls)
	}

	tap, err := s.app.FindFirstRecordByFilter(
		"whatsapp_messages", "message_id = {:id}", dbx.Params{"id": "wamid.tap-1"},
	)
	if err != nil {
		t.Fatalf("the tap is not in the conversation: %v", err)
	}
	if got := tap.GetString("message_content"); got != "Cancelar" {
		t.Errorf("message_content = %q, want the label the patient saw", got)
	}
	if got := tap.GetString("event_id"); got != appointmentID {
		t.Errorf("event_id = %q, want the appointment the tap moved", got)
	}
}

// The premise this ticket could not verify, failing: Meta turns out not to
// carry the reminder's id back on a tap. Nothing may be guessed at — no
// appointment moves — and the tap has to end up somewhere a person can see
// it, which is the thread, carrying no appointment.
func TestATapWithNoReplyContextMovesNothingAndIsStillInTheThread(t *testing.T) {
	s := newStack(t)

	appointmentID := saveAppointment(t, s.app, s.userID)
	saveReminder(t, s.app, s.userID, "wamid.reminder-1", appointmentID)

	body := tapDeliveryBody("wamid.tap-1", "5215511111111", "Cancelar", "", time.Now())
	if status := s.deliver(t, "/api/webhook", body, routerSecret, time.Now()); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	record, err := s.app.FindRecordById("appointments", appointmentID)
	if err != nil {
		t.Fatalf("read the appointment back: %v", err)
	}
	if got := record.GetString("status"); got != "pending" {
		t.Errorf("status = %q, want the pending it started as", got)
	}

	tap, err := s.app.FindFirstRecordByFilter(
		"whatsapp_messages", "message_id = {:id}", dbx.Params{"id": "wamid.tap-1"},
	)
	if err != nil {
		t.Fatalf("an unmatched tap was dropped instead of stored: %v", err)
	}
	if got := tap.GetString("event_id"); got != "" {
		t.Errorf("event_id = %q, want nothing — this tap matched nothing", got)
	}
	if got := tap.GetString("message_type"); got != "button" {
		t.Errorf("message_type = %q, want button so the thread can call it out", got)
	}
}
