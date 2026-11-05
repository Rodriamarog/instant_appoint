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

	meta *recorder

	// Two recorders for one stubbed service. One inbound message now
	// produces two different calls to Gemini — one that writes the reply and
	// one that reads the conversation for an appointment — and a count of
	// both together could not tell "it answered the patient" from "it read
	// the thread". They are told apart here by the tools the request
	// declares, which is what distinguishes them on the wire too.
	ai      *recorder
	reading *recorder

	reply string

	// recorded is the function call the stubbed model makes when it reads a
	// conversation. Nil is a conversation it found no appointment in, which
	// is what most of this file wants.
	recorded map[string]any
}

// recorder counts calls to a stubbed service and keeps the last body it saw.
type recorder struct {
	calls int
	paths []string
	last  map[string]any
}

func (r *recorder) record(req *http.Request) {
	raw, _ := io.ReadAll(req.Body)
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)

	r.recorded(req.URL.Path, body)
}

// recorded is the half of record that takes a body somebody else has already
// read — which the Gemini stub has to, because what it answers depends on
// what the request asked for.
func (r *recorder) recorded(path string, body map[string]any) {
	r.calls++
	r.paths = append(r.paths, path)
	r.last = body
}

func newStack(t *testing.T) *stack {
	t.Helper()

	s := &stack{
		app:     newApp(t),
		meta:    &recorder{},
		ai:      &recorder{},
		reading: &recorder{},
		reply:   "¿Te viene bien el martes a las 10?",
	}
	s.userID = connectAccount(t, s.app, "555000111", nil)
	// On autopilot, which is the only mode that sends anything and so the
	// only one in which the rest of this file has something to assert. The
	// clinic that has not switched the assistant on is the last test here.
	setAIMode(t, s.app, s.userID, "autopilot")

	// Gemini, answering with one candidate. Which of the two calls this is
	// is decided by the one thing that differs on the wire: the reading
	// declares a tool and the reply declares none.
	geminiStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		sent := map[string]any{}
		_ = json.Unmarshal(raw, &sent)

		part := map[string]any{"text": s.reply}
		if sent["tools"] != nil {
			s.reading.recorded(req.URL.Path, sent)
			if s.recorded != nil {
				part = map[string]any{"functionCall": map[string]any{
					"name": "record_appointment",
					"args": s.recorded,
				}}
			}
		} else {
			s.ai.recorded(req.URL.Path, sent)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{map[string]any{
				"content":      map[string]any{"role": "model", "parts": []any{part}},
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
		GenerateReply: func(ctx context.Context, channel webhook.Channel, history []webhook.ConversationMessage, text string) (string, error) {
			turns := make([]gemini.Message, 0, len(history))
			for _, turn := range history {
				turns = append(turns, gemini.Message{Role: turn.Role, Content: turn.Content})
			}

			if channel.ReachedThroughPage() {
				return ai.GenerateReplyWithoutANumber(ctx, turns, text)
			}

			return ai.GenerateReply(ctx, turns, text)
		},
		SendMessage: func(ctx context.Context, channel webhook.Channel, from, accessToken, to, text string) (string, error) {
			if channel.ReachedThroughPage() {
				return whatsApp.SendPageMessage(ctx, from, accessToken, to, text, nil)
			}

			return whatsApp.SendText(ctx, from, accessToken, to, text)
		},
		SendButtons: func(ctx context.Context, channel webhook.Channel, from, accessToken, to, body string, buttons []webhook.Button) (string, error) {
			if channel.ReachedThroughPage() {
				return whatsApp.SendPageMessage(ctx, from, accessToken, to, body, replyButtons(buttons))
			}

			return whatsApp.SendButtons(ctx, from, accessToken, to, body, replyButtons(buttons))
		},
		ExtractAppointment: func(ctx context.Context, history []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
			turns := make([]gemini.Message, 0, len(history))
			for _, turn := range history {
				turns = append(turns, gemini.Message{Role: turn.Role, Content: turn.Content})
			}

			read, err := ai.ExtractAppointment(ctx, turns)
			if read == nil || err != nil {
				return nil, err
			}

			return &webhook.ExtractedAppointment{
				ClientName:  read.ClientName,
				StartsAt:    read.StartsAt,
				Certain:     read.Certain,
				Summary:     read.Summary,
				ClientPhone: read.ClientPhone,
			}, nil
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
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
//
// Off is the position that costs nothing and does nothing: the patient's
// message is stored, because the clinic still reads its own conversations,
// and the proof that nothing else happened is that neither Gemini nor Meta
// was called at all. Observer is a different silence — it reads — and it has
// its own case below.
func TestADeliveryToAClinicWithTheAssistantOffIsStoredAndUntouched(t *testing.T) {
	s := newStack(t)
	setAIMode(t, s.app, s.userID, "off")
	now := time.Now()

	status := s.deliver(t, "/api/webhook", deliveryBody("wamid.1", "5215511111111", "hola", now), routerSecret, now)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if got := s.count(t, "whatsapp_messages"); got != 1 {
		t.Errorf("there are %d messages, want only the customer's", got)
	}
	if got := s.count(t, "conversations"); got != 1 {
		t.Errorf("there are %d conversations, want 1", got)
	}
	if s.ai.calls != 0 || s.reading.calls != 0 {
		t.Errorf("the model was called %d times to answer and %d to read, want none of either",
			s.ai.calls, s.reading.calls)
	}
	if s.meta.calls != 0 {
		t.Errorf("WhatsApp was called %d times, want none", s.meta.calls)
	}
	if got := s.count(t, "appointments"); got != 0 {
		t.Errorf("%d appointments were written with the assistant off, want none", got)
	}
}

// Observer, end to end: the clinic is holding the conversation itself, the
// patient hears nothing from this app, and the agreement reaches the
// calendar anyway. Until this ticket observer and off were the same thing.
func TestObserverWritesTheAppointmentThroughTheWholeStack(t *testing.T) {
	s := newStack(t)
	setAIMode(t, s.app, s.userID, "observer")
	s.recorded = map[string]any{
		"client_name": "Ana Ruiz",
		"starts_at":   "2026-10-30T15:00",
		"certain":     true,
		"summary":     "Limpieza dental",
	}
	now := time.Now()

	status := s.deliver(t, "/api/webhook", deliveryBody("wamid.1", "5215511111111", "el viernes a las 3", now), routerSecret, now)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	if s.ai.calls != 0 {
		t.Errorf("the model was asked for %d replies in observer mode, want none", s.ai.calls)
	}

	// One call, and it is the creation receipt rather than a reply.
	//
	// Observer promises that the assistant does not answer the patient's
	// messages, and it does not: the clinic is holding that conversation
	// from its own phone. The receipt is not an answer to anything — it is
	// the appointment's own message, the same family as the reminder that
	// goes out a day later whatever mode the clinic is in. An appointment
	// written out of a conversation nobody at the desk typed is exactly
	// the one worth checking.
	if s.meta.calls != 1 {
		t.Errorf("WhatsApp was called %d times in observer mode, want only the creation receipt", s.meta.calls)
	}
	if got, _ := s.meta.last["type"].(string); got != "interactive" {
		t.Errorf("the one message sent was of type %q, want the interactive receipt", got)
	}

	records, err := s.app.FindRecordsByFilter("appointments", "1=1", "", 0, 0)
	if err != nil {
		t.Fatalf("read the appointments back: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("stored %d appointments, want one", len(records))
	}

	for _, field := range []struct{ name, want string }{
		{"client_name", "Ana Ruiz"},
		{"client_phone", "5215511111111"},
		// 15:00 in Tijuana, written in the shape the rest of the app sorts
		// timestamps as.
		{"start_time", "2026-10-30T22:00:00.000Z"},
		{"status", "pending"},
		{"created_by", "assistant"},
	} {
		if got := records[0].GetString(field.name); got != field.want {
			t.Errorf("%s = %q, want %q", field.name, got, field.want)
		}
	}

	// The conversation it came out of, which is what answers "why does this
	// exist?" from the appointment itself.
	conversations, err := s.app.FindRecordsByFilter("conversations", "1=1", "", 0, 0)
	if err != nil {
		t.Fatalf("read the conversations back: %v", err)
	}
	if len(conversations) != 1 || records[0].GetString("conversation_id") != conversations[0].Id {
		t.Errorf("conversation_id = %q, want the conversation the message arrived in",
			records[0].GetString("conversation_id"))
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

// replyButtons copies the handler's buttons into the Graph client's, field
// by field, for the same reason the roles above are copied: neither package
// imports the other, so what a quick reply is stays the handler's decision
// and the wire shape stays the client's. cmd/server does the same.
func replyButtons(buttons []webhook.Button) []meta.Button {
	out := make([]meta.Button, 0, len(buttons))
	for _, button := range buttons {
		out = append(out, meta.Button{ID: button.ID, Title: button.Title})
	}

	return out
}

// receiptTapDeliveryBody is a tap on one of a creation receipt's reply
// buttons — the interactive shape, carrying the id this app set on the
// button rather than a template's label.
func receiptTapDeliveryBody(messageID, from, buttonID, title, repliesTo string, at time.Time) string {
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
						"id": messageID, "from": from, "type": "interactive",
						"interactive": map[string]any{
							"type":         "button_reply",
							"button_reply": map[string]any{"id": buttonID, "title": title},
						},
						"context":   map[string]any{"from": "555000111", "id": repliesTo},
						"timestamp": strconv.FormatInt(at.Unix(), 10),
					}},
				},
			}},
		}},
	})

	return string(payload)
}

// The whole of this ticket in one pass, and the one thing no unit test can
// prove: that the row written when the receipt is sent is the row the tap
// on it is matched through.
//
// Both halves are real here — the receipt is stored by this package and
// read back by this package — so a disagreement about what message_type a
// receipt carries, or about which types a tap may answer, fails here and
// nowhere else. Getting it wrong silently is the failure this test exists
// for: everything looks correct from the outside and every patient's answer
// lands on nothing.
func TestAReceiptsOwnTapConfirmsTheAppointmentEndToEnd(t *testing.T) {
	s := newStack(t)
	setAIMode(t, s.app, s.userID, "observer")
	s.recorded = map[string]any{
		"client_name": "Ana Ruiz",
		"starts_at":   "2026-10-30T15:00",
		"certain":     true,
		"summary":     "Limpieza dental",
	}
	now := time.Now()

	status := s.deliver(t, "/api/webhook", deliveryBody("wamid.1", "5215511111111", "el viernes a las 3", now), routerSecret, now)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	// The wamid the Meta stub handed back for the receipt, which is what
	// Meta puts in the context of a tap on it.
	receiptID := "wamid.reply-1"
	receipt, err := s.app.FindFirstRecordByFilter(
		"whatsapp_messages", "message_id = {:id}", dbx.Params{"id": receiptID},
	)
	if err != nil {
		t.Fatalf("the receipt left no row behind, so no tap on it could ever be matched: %v", err)
	}
	if receipt.GetString("event_id") == "" {
		t.Fatal("the receipt's row names no appointment; a tap on it would arrive matching nothing")
	}

	body := receiptTapDeliveryBody("wamid.tap-1", "5215511111111", "confirm", "Confirmar", receiptID, time.Now())
	if status := s.deliver(t, "/api/webhook", body, routerSecret, time.Now()); status != http.StatusOK {
		t.Fatalf("the tap: status = %d, want 200", status)
	}

	record, err := s.app.FindRecordById("appointments", receipt.GetString("event_id"))
	if err != nil {
		t.Fatalf("read the appointment back: %v", err)
	}
	if got := record.GetString("status"); got != "confirmed" {
		t.Errorf("status = %q, want confirmed", got)
	}
	if got := record.GetString("status_set_by"); got != "patient" {
		t.Errorf("status_set_by = %q, want patient", got)
	}
}
