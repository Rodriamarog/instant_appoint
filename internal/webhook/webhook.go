// Package webhook turns a WhatsApp webhook delivery into a stored message and,
// when it should, an AI reply.
//
// It is a plain http.Handler over two interfaces — a Store it persists through
// and a few injected functions — and it imports no database driver and no
// network client. That is deliberate and enforced by a test in internal/arch:
// this is the one piece of the app whose behaviour is defined by a long list
// of cases Meta can produce, and it is only cheap to keep those cases honest
// while the whole suite runs with no database and no network.
//
// The PocketBase-backed Store lives in internal/pbstore, and cmd/server is
// where the two meet.
//
// # What arrives here
//
// Meta never reaches this app directly. Deliveries come through the Neurocrow
// webhook router, which answers Meta's verification handshake, checks Meta's
// signature, and re-signs each delivery with a shared secret — see
// internal/routersig. The router forwards every number it serves, most of
// which belong to other apps behind it, so an unknown phone number id is the
// ordinary case and not an error.
//
// The router retries on any non-2xx, so the status code is a control signal:
// a storage failure must answer non-2xx to get the message redelivered, and
// everything else must answer 200 or the same delivery arrives forever.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/internal/routersig"
)

// MaxAutoReplyAge is how old a message may be, by its own WhatsApp timestamp,
// and still get an automatic reply.
//
// A backlog delivered after an outage is history rather than a conversation:
// answering it means the bot replying to questions hours after they were
// asked, often after a human already did. Such messages are still stored, so
// the dashboard shows them.
const MaxAutoReplyAge = 15 * time.Minute

// maxBodyBytes bounds what this handler will read. Meta's deliveries are a few
// kilobytes; this is three orders of magnitude of headroom and still keeps an
// unauthenticated public endpoint from being used to exhaust memory. The limit
// is applied before the signature is checked, because the signature cannot be
// checked without reading the body first.
const maxBodyBytes = 1 << 20

// Direction says who sent a message.
type Direction string

const (
	// Inbound is a message from the customer.
	Inbound Direction = "inbound"

	// Outbound is a message from the business: either this app's AI reply or
	// an echo of something a human sent from the WhatsApp Business app.
	Outbound Direction = "outbound"
)

// The roles Gemini understands. A conversation is told to the model as the
// customer's turns and the business's turns, so an echo of a human reply reads
// to the model exactly like one of its own.
const (
	RoleUser  = "user"
	RoleModel = "model"
)

// StoredMessage is one row of the conversation.
type StoredMessage struct {
	// MessageID is WhatsApp's own id for the message, which is what makes a
	// redelivery recognisable.
	MessageID string

	ConversationID string

	// CustomerPhone is the customer's number in both directions — the sender
	// of an inbound message and the recipient of an outbound one.
	CustomerPhone string

	Content   string
	Direction Direction

	// SentAt is when this app received or sent the message, not Meta's
	// timestamp, in the format internal/isotime defines.
	SentAt string
}

// Account is the connected WhatsApp number a delivery belongs to, together
// with what the clinic behind it has allowed the assistant to do.
type Account struct {
	UserID      string
	AccessToken string

	// Mode is the clinic's ai_mode setting. It rides along with the account
	// rather than arriving through a Store method of its own because the
	// handler wants both for the same message and the account lookup has
	// already resolved the clinic by the time the mode is needed.
	//
	// A clinic that has never saved any settings has no row to read it off,
	// and the zero value is what that looks like here — which is not
	// ModeAutopilot, so nothing is sent.
	Mode Mode
}

// ConversationMessage is one turn of the history handed to the AI.
type ConversationMessage struct {
	// Role is RoleUser or RoleModel.
	Role    string
	Content string
}

// Store is the persistence the handler needs. Production is PocketBase
// (internal/pbstore); the tests are an in-memory fake.
type Store interface {
	// FindAccount returns the connected account for a phone number id, or a
	// nil Account and a nil error when no account has that number. Not found
	// is not an error: most numbers the router forwards belong elsewhere.
	FindAccount(ctx context.Context, phoneNumberID string) (*Account, error)

	// HasMessage reports whether a message with this WhatsApp id is already
	// stored.
	HasMessage(ctx context.Context, messageID string) (bool, error)

	// UpsertConversation returns the id of the customer's active conversation,
	// creating it if there is none, and records at as the time of its latest
	// message.
	UpsertConversation(ctx context.Context, userID, customerPhone, phoneNumberID, at string) (string, error)

	// SaveMessage stores one message. An implementation may treat a message
	// that is already stored as a no-op, which is what makes a redelivery
	// racing an in-flight delivery safe.
	SaveMessage(ctx context.Context, userID string, message StoredMessage) error

	// ConversationHistory returns every message of a conversation, oldest
	// first.
	ConversationHistory(ctx context.Context, conversationID string) ([]StoredMessage, error)

	// HasAppointmentFromConversation reports whether an appointment already
	// records this conversation as where it came from.
	HasAppointmentFromConversation(ctx context.Context, conversationID string) (bool, error)

	// CreateAppointment writes a new appointment for a clinic.
	//
	// Creating one is the only thing this interface lets the assistant do to
	// an appointment, and that is the design rather than an omission. A
	// phantom appointment is visible, odd and recoverable; a real
	// appointment deleted because somebody wrote "I might not make it" takes
	// the time, the phone number and the reason with it, and nobody ever
	// learns it was there. A rule written into a prompt is a rule the model
	// can misread, and a method that is not on this interface is one the
	// handler cannot call however the conversation goes.
	CreateAppointment(ctx context.Context, userID string, appointment Appointment) error
}

// Deps is everything the handler reaches outside itself.
type Deps struct {
	// Secret is the shared secret the Neurocrow router signs with. An empty
	// Secret rejects every delivery rather than accepting every delivery, so a
	// deployment that forgot to configure it fails closed.
	Secret string

	// Now is the only clock. Tests freeze it.
	Now func() time.Time

	Store Store

	// GenerateReply writes the reply to text, given the conversation so far
	// with text itself excluded.
	GenerateReply func(ctx context.Context, history []ConversationMessage, text string) (string, error)

	// SendMessage sends text to the customer and returns WhatsApp's id for it.
	SendMessage func(ctx context.Context, phoneNumberID, accessToken, to, text string) (string, error)

	// ExtractAppointment reads a whole conversation and reports the
	// appointment it agreed on, or nil when it agreed none.
	//
	// It is a second call rather than something the reply hands back,
	// because observer mode writes no reply at all: the clinic is holding
	// the conversation itself from a phone, and there is nothing for an
	// extraction to ride along on.
	ExtractAppointment func(ctx context.Context, history []ConversationMessage) (*ExtractedAppointment, error)

	// Logger may be nil, in which case slog's default is used.
	Logger *slog.Logger
}

// New returns the webhook handler.
//
// Mount it on both /api/webhook and /webhook, and mount one handler rather
// than one per path: the in-flight guard lives in the returned handler, and
// two handlers would not see each other's work.
func New(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	return &handler{
		deps: deps,
		// Message ids being processed right now. The store's duplicate check
		// cannot close the window between "this message is not stored" and
		// "this message is now stored", and the router can have two deliveries
		// of one message in flight at once.
		inFlight: map[string]struct{}{},
	}
}

type handler struct {
	deps Deps

	mu       sync.Mutex
	inFlight map[string]struct{}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The body is read whole and first, before anything parses it: the
	// signature covers these exact bytes, and a payload that has been through
	// a decoder and an encoder is a different sequence of bytes that means the
	// same thing — and verifies against nothing.
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		h.fail(w, http.StatusBadRequest, "The delivery could not be read.")

		return
	}
	if len(raw) > maxBodyBytes {
		h.fail(w, http.StatusRequestEntityTooLarge, "The delivery is too large.")

		return
	}

	if !routersig.Valid(h.deps.Secret, raw, r.Header, h.deps.Now()) {
		h.fail(w, http.StatusUnauthorized, "The delivery is not signed by the webhook router.")

		return
	}

	// Decoded loosely rather than into a struct: this is untrusted input, and a
	// payload whose shape surprises a strict decoder must be skipped, not
	// answered with an error that makes the router retry it forever.
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		h.fail(w, http.StatusBadRequest, "The delivery is not valid JSON.")

		return
	}

	for _, message := range extractTexts(payload) {
		if !h.claim(message.MessageID) {
			continue
		}

		err := h.process(r.Context(), message)
		h.release(message.MessageID)

		if err != nil {
			// A non-2xx is the only way to ask the router to deliver this
			// again. Messages after this one in the same payload are left
			// unprocessed on purpose: the retry carries the whole delivery,
			// and deduplication makes re-processing the earlier ones a no-op.
			h.deps.Logger.Error("storing a webhook message failed; the delivery will be retried",
				"message_id", message.MessageID, "error", err)
			h.fail(w, http.StatusInternalServerError, "The delivery could not be processed.")

			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// fail answers in PocketBase's error shape, so a failure from this route and a
// failure from any other route in this app parse the same way. The message is
// deliberately free of detail: the only caller is the router, which logs it.
func (h *handler) fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  status,
		"message": message,
		"data":    map[string]any{},
	})
}

// claim reserves a message id for processing, reporting false when another
// delivery of the same message holds it.
func (h *handler) claim(messageID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, taken := h.inFlight[messageID]; taken {
		return false
	}
	h.inFlight[messageID] = struct{}{}

	return true
}

func (h *handler) release(messageID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.inFlight, messageID)
}

// process stores one message and, if it deserves one, answers it.
//
// A returned error means the message was not stored and the delivery should be
// retried. Everything else — an unknown number, a duplicate, a failed AI step
// — returns nil, because retrying would not help.
func (h *handler) process(ctx context.Context, message incomingText) error {
	store := h.deps.Store

	stored, err := store.HasMessage(ctx, message.MessageID)
	if err != nil {
		return fmt.Errorf("look up message %s: %w", message.MessageID, err)
	}
	if stored {
		h.deps.Logger.Info("ignored a redelivered message", "message_id", message.MessageID)

		return nil
	}

	account, err := store.FindAccount(ctx, message.PhoneNumberID)
	if err != nil {
		return fmt.Errorf("look up the account for phone number id %s: %w", message.PhoneNumberID, err)
	}
	if account == nil {
		// The router forwards every number it serves. One this app does not
		// know is somebody else's, not a fault.
		h.deps.Logger.Warn("no connected account for the delivered phone number id",
			"phone_number_id", message.PhoneNumberID)

		return nil
	}

	// One timestamp for both writes, so the conversation's last_message_at and
	// its newest message agree exactly. It is the receive time, not Meta's: a
	// message delayed in Meta's queue would otherwise sort into the middle of
	// a conversation it arrived at the end of.
	now := isotime.Format(h.deps.Now())

	conversationID, err := store.UpsertConversation(ctx, account.UserID, message.CustomerPhone, message.PhoneNumberID, now)
	if err != nil {
		return fmt.Errorf("upsert the conversation for %s: %w", message.CustomerPhone, err)
	}

	err = store.SaveMessage(ctx, account.UserID, StoredMessage{
		MessageID:      message.MessageID,
		ConversationID: conversationID,
		CustomerPhone:  message.CustomerPhone,
		Content:        message.Content,
		Direction:      message.Direction,
		SentAt:         now,
	})
	if err != nil {
		return fmt.Errorf("store message %s: %w", message.MessageID, err)
	}

	// From here on nothing is retryable. The message is stored, which is the
	// part that must not be lost; a failure in either AI step below is logged
	// and the delivery is still acknowledged, because a retry would re-run
	// that step against a message the store already has.

	// An echo is the business's own message, captured so the dashboard shows
	// the whole conversation. Replying would mean the bot answering the
	// business — but reading it is a different matter, which is why the
	// extraction below is not inside this branch.
	if message.Direction == Inbound {
		if reason := h.notAnswerable(*account, message); reason != "" {
			h.deps.Logger.Info("no auto-reply: "+reason, "message_id", message.MessageID, "mode", string(account.Mode))
		} else if err := h.reply(ctx, *account, conversationID, message); err != nil {
			h.deps.Logger.Error("the AI reply step failed; the message is stored and the delivery is acknowledged",
				"message_id", message.MessageID, "error", err)
		}
	}

	// After the reply and not before it. On autopilot the assistant's own
	// answer is often the half that settles the appointment, and it is
	// stored by the step above — so reading the conversation first would
	// read it a turn short and find nothing to write.
	if account.Mode.RecordsAppointments() {
		if err := h.recordAppointment(ctx, *account, conversationID); err != nil {
			h.deps.Logger.Error("reading the conversation for an appointment failed; the message is stored and the delivery is acknowledged",
				"conversation_id", conversationID, "error", err)
		}
	}

	return nil
}

// notAnswerable says why this message gets no automatic reply, or "" when it
// gets one.
//
// Neither of the two age checks guards the extraction as well, deliberately.
// They exist so the bot does not speak into a conversation that has moved on
// — answering a question a human already answered hours ago. Writing down an
// agreement that was reached hours ago is not the same act and is still worth
// doing: the clinic wants the appointment either way, and the duplicate rule
// bounds a replayed backlog to one appointment per conversation.
func (h *handler) notAnswerable(account Account, message incomingText) string {
	switch {
	case !account.Mode.RepliesAutomatically():
		return "the assistant is not on autopilot for this clinic"

	case !message.HasTimestamp:
		// Without a timestamp the age cannot be established, and a message
		// that might be a day old is not one to answer automatically.
		return "the message carried no timestamp"

	case h.deps.Now().Sub(message.Timestamp) > MaxAutoReplyAge:
		return "the message is too old"
	}

	return ""
}

// recordAppointment reads the conversation and writes the appointment it
// agreed on, if it agreed one and has not already produced one.
func (h *handler) recordAppointment(ctx context.Context, account Account, conversationID string) error {
	// The duplicate rule, and the first thing in the path rather than the
	// last.
	//
	// The model is shown the whole conversation every time, so the agreement
	// reached in the first message is still there in the fortieth — and a
	// conversation carries on after an agreement, as every real one does.
	// Without this, one chat leaves the clinic with forty copies of one
	// appointment and no way to tell which is the real one.
	//
	// One per conversation, rather than one per agreed time: a second
	// appointment arranged in the same thread is something a member of staff
	// can add in a few seconds from a proposal they can see, and forty
	// phantom rows is not something anybody can clean up. It is checked
	// before the model because the model is the expensive half, and a thread
	// that runs for fifty messages after an agreement must not be read fifty
	// times.
	already, err := h.deps.Store.HasAppointmentFromConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("look for an appointment already written from this conversation: %w", err)
	}
	if already {
		return nil
	}

	stored, err := h.deps.Store.ConversationHistory(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("read the conversation history: %w", err)
	}

	// The whole conversation, newest turn included — unlike the reply, which
	// holds the triggering message back as the turn being answered. Nothing
	// is being answered here, and the newest turn is the one most likely to
	// carry the agreement.
	history := make([]ConversationMessage, 0, len(stored))
	for _, m := range stored {
		role := RoleModel
		if m.Direction == Inbound {
			role = RoleUser
		}
		history = append(history, ConversationMessage{Role: role, Content: m.Content})
	}

	extracted, err := h.deps.ExtractAppointment(ctx, history)
	if err != nil {
		return fmt.Errorf("read the conversation for an appointment: %w", err)
	}
	if extracted == nil {
		// Most conversations are a question about the opening hours. None of
		// them may leave an appointment behind.
		return nil
	}

	// The customer's number, taken from the conversation rather than from
	// anything the model wrote: it is the one fact here that is not an
	// interpretation.
	customerPhone := ""
	if len(stored) > 0 {
		customerPhone = stored[len(stored)-1].CustomerPhone
	}

	appointment := extracted.Appointment(customerPhone, conversationID, h.deps.Now())
	if err := h.deps.Store.CreateAppointment(ctx, account.UserID, appointment); err != nil {
		return fmt.Errorf("write the appointment: %w", err)
	}

	h.deps.Logger.Info("wrote an appointment out of a conversation",
		"conversation_id", conversationID, "status", string(appointment.Status), "starts_at", appointment.StartsAt)

	return nil
}

// reply generates, sends and stores the answer to one inbound message.
func (h *handler) reply(ctx context.Context, account Account, conversationID string, message incomingText) error {
	stored, err := h.deps.Store.ConversationHistory(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("read the conversation history: %w", err)
	}

	// The message being answered was written above, so it is already in the
	// history. It is passed to the model separately, as the turn to answer, so
	// it is filtered out here rather than appearing twice.
	history := make([]ConversationMessage, 0, len(stored))
	for _, m := range stored {
		if m.MessageID == message.MessageID {
			continue
		}
		role := RoleModel
		if m.Direction == Inbound {
			role = RoleUser
		}
		history = append(history, ConversationMessage{Role: role, Content: m.Content})
	}

	text, err := h.deps.GenerateReply(ctx, history, message.Content)
	if err != nil {
		return fmt.Errorf("generate a reply: %w", err)
	}

	replyID, err := h.deps.SendMessage(ctx, message.PhoneNumberID, account.AccessToken, message.CustomerPhone, text)
	if err != nil {
		return fmt.Errorf("send the reply: %w", err)
	}

	// A second clock reading: in production the AI and the send take seconds,
	// and the reply did not happen when the message arrived.
	err = h.deps.Store.SaveMessage(ctx, account.UserID, StoredMessage{
		MessageID:      replyID,
		ConversationID: conversationID,
		CustomerPhone:  message.CustomerPhone,
		Content:        text,
		Direction:      Outbound,
		SentAt:         isotime.Format(h.deps.Now()),
	})
	if err != nil {
		return fmt.Errorf("store the reply: %w", err)
	}

	h.deps.Logger.Info("sent an AI reply", "to", message.CustomerPhone, "message_id", replyID)

	return nil
}
