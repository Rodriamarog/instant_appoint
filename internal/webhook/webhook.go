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

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
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

// MessageKind is how a message was produced, which is the difference between
// a patient typing "Confirmar" and a patient tapping a button that says it.
// The two read the same in a thread and are not the same thing: only the
// second is one this app acted on, and only the second can fail to find the
// appointment it was about.
type MessageKind string

const (
	// KindText is a message somebody typed. It is the zero value, so a caller
	// written before taps existed keeps storing exactly what it stored before.
	KindText MessageKind = ""

	// KindTap is a tap on a quick-reply button.
	KindTap MessageKind = "button"
)

// StoredMessage is one row of the conversation.
type StoredMessage struct {
	// MessageID is WhatsApp's own id for the message, which is what makes a
	// redelivery recognisable.
	MessageID string

	// Kind is how the message was produced.
	Kind MessageKind

	// AppointmentID is the appointment a tap was matched to and moved, empty
	// on everything else — including a tap nothing could be matched to.
	//
	// Empty on a tap is therefore not missing information: it is the record
	// that this tap changed nothing, and it is what the thread reads to say
	// so. Written only once the status change has gone through, so a row
	// naming an appointment is a row that moved it.
	AppointmentID string

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

	// AppointmentForReminder returns the appointment a sent reminder was
	// about, given Meta's id for that reminder, or "" when this clinic has no
	// reminder with that id.
	//
	// Not found is not an error: a tap can answer any message the business
	// ever sent, and most of them are not reminders. The clinic is a
	// parameter and not a convenience — a wamid matched across clinics would
	// let one clinic's delivery move another clinic's appointment.
	AppointmentForReminder(ctx context.Context, userID, reminderMessageID string) (string, error)

	// SetAppointmentStatus moves an appointment and records who moved it and
	// when, in the vocabulary of internal/appointments.
	//
	// It is called before the tap that caused it is stored, so that a failure
	// here leaves the delivery retryable: once the tap is stored the handler
	// treats a redelivery as a duplicate, and the status change would have
	// nothing left to re-attempt it.
	SetAppointmentStatus(ctx context.Context, appointmentID string, status appointments.Status, by appointments.Actor, at string) error
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

	for _, message := range extractMessages(payload) {
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
func (h *handler) process(ctx context.Context, message incomingMessage) error {
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

	// Acted on before it is stored, deliberately. The tap is the trigger and
	// the row is the record of it, and once the row exists a redelivery is a
	// duplicate the handler skips — so a status change attempted after the
	// write and failing there would never be attempted again, leaving a
	// patient who cancelled still booked and nothing anywhere saying so.
	// This way a failure is a 500, the router redelivers, and setting the
	// same status twice costs a restamped status_set_at.
	//
	// Only an inbound tap: an echo is the business's own message coming back,
	// and a button in it was not pressed by the patient.
	appointmentID := ""
	if message.Kind == KindTap && message.Direction == Inbound {
		appointmentID, err = h.applyTap(ctx, account.UserID, message, now)
		if err != nil {
			return err
		}
	}

	err = store.SaveMessage(ctx, account.UserID, StoredMessage{
		MessageID:      message.MessageID,
		Kind:           message.Kind,
		AppointmentID:  appointmentID,
		ConversationID: conversationID,
		CustomerPhone:  message.CustomerPhone,
		Content:        message.Content,
		Direction:      message.Direction,
		SentAt:         now,
	})
	if err != nil {
		return fmt.Errorf("store message %s: %w", message.MessageID, err)
	}

	// An echo is the business's own message, captured so the dashboard shows
	// the whole conversation. Replying would mean the bot answering the
	// business.
	if message.Direction != Inbound {
		return nil
	}

	// A tap is an answer, not a question. The patient has already been told
	// what the reminder said and has pressed one of the two buttons under it;
	// an assistant replying to that is a second conversation nobody asked
	// for, and one that would have to describe a cancellation it did not
	// make. The row is stored above, so the model sees the tap in the history
	// next time the patient does write.
	if message.Kind == KindTap {
		return nil
	}

	// Checked before the age and before the model, so that a clinic which has
	// not switched the assistant on costs nothing and says nothing. The
	// message above is already stored, which is the half that happens in
	// every mode.
	if !account.Mode.RepliesAutomatically() {
		h.deps.Logger.Info("no auto-reply: the assistant is not on autopilot for this clinic",
			"message_id", message.MessageID, "mode", string(account.Mode))

		return nil
	}

	if !message.HasTimestamp {
		// Without a timestamp the age cannot be established, and a message
		// that might be a day old is not one to answer automatically.
		h.deps.Logger.Info("no auto-reply: the message carried no timestamp", "message_id", message.MessageID)

		return nil
	}
	if age := h.deps.Now().Sub(message.Timestamp); age > MaxAutoReplyAge {
		h.deps.Logger.Info("no auto-reply: the message is too old", "message_id", message.MessageID, "age", age)

		return nil
	}

	// From here on nothing is retryable. The message is stored, which is the
	// part that must not be lost; a failed reply is logged and the delivery is
	// still acknowledged, because a retry would re-run the AI step against a
	// message the store already has.
	if err := h.reply(ctx, *account, conversationID, message); err != nil {
		h.deps.Logger.Error("the AI reply step failed; the message is stored and the delivery is acknowledged",
			"message_id", message.MessageID, "error", err)
	}

	return nil
}

// applyTap moves the appointment a tap was about and returns its id, or ""
// when the tap moved nothing.
//
// Returning "" is an outcome, not a failure. Three things have to line up —
// the delivery has to say which message the tap answers, that message has to
// be a reminder this clinic sent, and the button has to be one this app can
// name — and none of them is something this app controls. Every way they can
// fail to line up is logged here and leaves the tap stored with no
// appointment against it, which is what puts it in front of somebody in the
// thread. The alternative, guessing at the appointment, is a cancellation
// applied to the wrong patient.
//
// Only a real storage failure is returned, because only that is worth
// redelivering.
func (h *handler) applyTap(ctx context.Context, userID string, message incomingMessage, now string) (string, error) {
	if message.RepliesTo == "" {
		h.deps.Logger.Warn("a tap arrived naming no message it answers; no appointment was changed",
			"message_id", message.MessageID, "content", message.Content)

		return "", nil
	}

	appointmentID, err := h.deps.Store.AppointmentForReminder(ctx, userID, message.RepliesTo)
	if err != nil {
		return "", fmt.Errorf("look up the reminder %s a tap answers: %w", message.RepliesTo, err)
	}
	if appointmentID == "" {
		h.deps.Logger.Warn("a tap answers a message that is not a reminder this clinic sent; no appointment was changed",
			"message_id", message.MessageID, "replies_to", message.RepliesTo)

		return "", nil
	}

	action, ok := resolveTap(message.TapChoices)
	if !ok {
		h.deps.Logger.Warn("a tap named a button this app does not offer; no appointment was changed",
			"message_id", message.MessageID, "appointment_id", appointmentID, "content", message.Content)

		return "", nil
	}

	err = h.deps.Store.SetAppointmentStatus(ctx, appointmentID, action.Status(), appointments.Patient, now)
	if err != nil {
		return "", fmt.Errorf("record the patient's tap against appointment %s: %w", appointmentID, err)
	}

	h.deps.Logger.Info("a patient's tap moved an appointment",
		"message_id", message.MessageID, "appointment_id", appointmentID, "status", string(action.Status()))

	return appointmentID, nil
}

// reply generates, sends and stores the answer to one inbound message.
func (h *handler) reply(ctx context.Context, account Account, conversationID string, message incomingMessage) error {
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
