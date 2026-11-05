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

	// KindReceipt is the message this app sends the moment the assistant
	// writes an appointment, asking the patient to confirm it.
	//
	// It is a kind of its own and not a reminder, although it carries the
	// same two buttons and the same appointment. internal/reminderjob
	// counts a sent reminder row as the reminder having gone out, and
	// reserves the send with a unique index predicated on that spelling —
	// so a receipt filed as one would silently cancel the real reminder
	// for that appointment.
	KindReceipt MessageKind = "receipt"
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

	// Channel is the app the message went through. The zero value is stored
	// as WhatsApp by the column's default.
	Channel Channel

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

	// SendFrom is what a reply is sent from when it is not the id the
	// delivery was addressed to: the Facebook Page an Instagram account's
	// messages go out through. Empty means the delivery's own id, which is
	// right for WhatsApp and Messenger.
	SendFrom string

	// Mode is the clinic's ai_mode setting. It rides along with the account
	// rather than arriving through a Store method of its own because the
	// handler wants both for the same message and the account lookup has
	// already resolved the clinic by the time the mode is needed.
	//
	// A clinic that has never saved any settings has no row to read it off,
	// and the zero value is what that looks like here — which is not
	// ModeAutopilot, so nothing is sent.
	Mode Mode

	// Outreach is the clinic's patient_outreach setting: whether the
	// assistant may message a patient who has not just messaged it. It
	// rides along with the account for the same reason Mode does, and it
	// is a second field rather than a fourth position on Mode because it
	// answers a different question — see outreach.go.
	//
	// The zero value is what a clinic with no settings row looks like,
	// and here that reads as on, which is the behaviour such a clinic
	// already had.
	Outreach Outreach

	// Timezone is the IANA zone the clinic keeps time in, which is the
	// clock a message to a patient names an hour on. Appointments are
	// stored as UTC instants, so a receipt written without it would send
	// somebody in at ten at night.
	Timezone string

	// SendsReminders is whether a reminder will actually go out for this
	// clinic's appointments: switched on and with a template chosen. Both
	// halves, because either one missing means no reminder is ever sent and
	// so no receipt of theirs may be held back in favour of one.
	SendsReminders bool

	// ReminderLeadTime is how far before an appointment that reminder falls
	// due. An appointment booked inside it has its reminder due already,
	// which is what suppresses the receipt — see handler.suppressionReason.
	ReminderLeadTime time.Duration
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
	// FindAccount returns the connected account a delivery was addressed
	// to — a WhatsApp phone number id, a Page id or an Instagram account id,
	// by channel — or a nil Account and a nil error when none has it. Not
	// found is not an error: most ids the router forwards belong elsewhere.
	FindAccount(ctx context.Context, channel Channel, key string) (*Account, error)

	// HasMessage reports whether a message with this WhatsApp id is already
	// stored.
	HasMessage(ctx context.Context, messageID string) (bool, error)

	// UpsertConversation returns the id of the customer's active conversation,
	// creating it if there is none, and records at as the time of its latest
	// message.
	UpsertConversation(ctx context.Context, userID string, channel Channel, customerPhone, accountKey, at string) (string, error)

	// SaveMessage stores one message. An implementation may treat a message
	// that is already stored as a no-op, which is what makes a redelivery
	// racing an in-flight delivery safe.
	SaveMessage(ctx context.Context, userID string, message StoredMessage) error

	// ConversationHistory returns every message of a conversation, oldest
	// first.
	ConversationHistory(ctx context.Context, conversationID string) ([]StoredMessage, error)

	AppointmentWriter

	// AppointmentForSentMessage returns the appointment a message this app
	// sent was about, given Meta's id for that message, or "" when this
	// clinic sent no such message.
	//
	// Two kinds of message qualify: a reminder, and the receipt that goes
	// out the moment the assistant writes an appointment. Both ask the same
	// question with the same two buttons, so a tap on either has to find
	// its way back the same way.
	//
	// Not found is not an error: a tap can answer any message the business
	// ever sent, and most of them ask nothing. The clinic is a parameter
	// and not a convenience — a wamid matched across clinics would let one
	// clinic's delivery move another clinic's appointment.
	AppointmentForSentMessage(ctx context.Context, userID, sentMessageID string) (string, error)

	// AppointmentInConversation reports whether an appointment of this
	// clinic's came out of this conversation.
	//
	// It is how a Messenger or Instagram tap is checked. There, the receipt's
	// buttons carry the appointment's id in their payload — those channels
	// report no id for the message a quick reply answers — and a payload is
	// only as trustworthy as the check that the appointment it names belongs
	// to the patient who tapped it.
	AppointmentInConversation(ctx context.Context, userID, conversationID, appointmentID string) (bool, error)

	// SetAppointmentStatus moves an appointment and records who moved it and
	// when, in the vocabulary of internal/appointments.
	//
	// It is called before the tap that caused it is stored, so that a failure
	// here leaves the delivery retryable: once the tap is stored the handler
	// treats a redelivery as a duplicate, and the status change would have
	// nothing left to re-attempt it.
	SetAppointmentStatus(ctx context.Context, appointmentID string, status appointments.Status, by appointments.Actor, at string) error
}

// AppointmentWriter is everything the assistant may do to an appointment.
//
// It is an interface of its own, and the step that reads conversations is
// handed this rather than the whole Store, so the asymmetry the feature
// turns on is the compiler's business and not a promise. The assistant may
// create an appointment and may never move or delete one: a phantom
// appointment is visible, odd and recoverable, while a real appointment
// deleted because a patient wrote "I might not make it" is gone along with
// the time, the number and the reason, and nobody ever learns it was there.
//
// Store also carries SetAppointmentStatus, and that is not a hole in this.
// That path belongs to a patient pressing Confirmar or Cancelar on a
// reminder this app sent - a deliberate answer, matched to one appointment
// by the id Meta gave that reminder, and recorded as the patient's own
// doing. What must never happen is the model reading a cancellation out of
// free text and acting on it, and the step that reads free text cannot
// reach that method from here.
type AppointmentWriter interface {
	// HasAppointmentFromConversation reports whether an appointment already
	// records this conversation as where it came from.
	HasAppointmentFromConversation(ctx context.Context, conversationID string) (bool, error)

	// CreateAppointment writes a new appointment for a clinic and returns
	// its id.
	//
	// An empty id with a nil error means no row was written: the duplicate
	// rule's unique index refused this one because another delivery of the
	// same conversation got there first. That is an outcome and not a
	// failure — the appointment that won is the right one — but the two
	// have to be told apart here, because everything this handler does
	// afterwards is about an appointment it just created. A receipt sent
	// for somebody else's write is the duplicate message the receipt exists
	// to prevent.
	CreateAppointment(ctx context.Context, userID string, appointment Appointment) (string, error)
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
	// with text itself excluded. The channel is passed so the reply can ask
	// for what that channel does not already know — a phone number, on
	// Messenger and Instagram.
	GenerateReply func(ctx context.Context, channel Channel, history []ConversationMessage, text string) (string, error)

	// SendMessage sends text to the customer over channel and returns Meta's
	// id for it. from is the WhatsApp phone number id or the Page id.
	SendMessage func(ctx context.Context, channel Channel, from, accessToken, to, text string) (string, error)

	// SendButtons sends body to the customer with quick replies under it
	// and returns WhatsApp's id for it.
	//
	// It is free-form and not a template, which is only possible because
	// the conversation that produced the appointment has just happened:
	// inside the 24-hour window a customer's own message opens, Meta
	// delivers anything. A reminder a day later has no such window and must
	// be a template Meta has approved, which is why the two paths do not
	// share a send.
	SendButtons func(ctx context.Context, channel Channel, from, accessToken, to, body string, buttons []Button) (string, error)

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

	account, err := store.FindAccount(ctx, message.Channel, message.AccountKey)
	if err != nil {
		return fmt.Errorf("look up the %s account %s: %w", message.Channel, message.AccountKey, err)
	}
	if account == nil {
		// The router forwards every number it serves. One this app does not
		// know is somebody else's, not a fault.
		h.deps.Logger.Warn("no connected account for the delivered id",
			"channel", string(message.Channel), "id", message.AccountKey)

		return nil
	}

	// One timestamp for both writes, so the conversation's last_message_at and
	// its newest message agree exactly. It is the receive time, not Meta's: a
	// message delayed in Meta's queue would otherwise sort into the middle of
	// a conversation it arrived at the end of.
	now := isotime.Format(h.deps.Now())

	conversationID, err := store.UpsertConversation(ctx, account.UserID, message.Channel, message.CustomerPhone, message.AccountKey, now)
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
		appointmentID, err = h.applyTap(ctx, account.UserID, conversationID, message, now)
		if err != nil {
			return err
		}
	}

	err = store.SaveMessage(ctx, account.UserID, StoredMessage{
		MessageID:      message.MessageID,
		Kind:           message.Kind,
		AppointmentID:  appointmentID,
		ConversationID: conversationID,
		Channel:        message.Channel,
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
	// business - but reading it is a different matter, which is why the
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
	// stored by the step above - so reading the conversation first would read
	// it a turn short and find nothing to write.
	//
	// A tap is left out for the same reason it gets no reply: it is an
	// answer to a question this app asked, it has already been acted on
	// above, and there is no agreement in it to read.
	if message.Kind != KindTap && account.Mode.RecordsAppointments() {
		created, err := h.recordAppointment(ctx, h.deps.Store, *account, message.Channel, conversationID)
		if err != nil {
			h.deps.Logger.Error("reading the conversation for an appointment failed; the message is stored and the delivery is acknowledged",
				"conversation_id", conversationID, "error", err)
		}
		if created != nil {
			h.checkWithThePatient(ctx, *account, message, conversationID, *created)
		}
	}

	return nil
}

// notAnswerable says why this message gets no automatic reply, or "" when it
// gets one.
//
// Neither age check guards the extraction as well, deliberately. They exist
// so the bot does not speak into a conversation that has moved on -
// answering a question a human already answered hours ago. Writing down an
// agreement reached hours ago is not the same act and is still worth doing:
// the clinic wants the appointment either way, and the duplicate rule bounds
// a replayed backlog to one appointment per conversation.
func (h *handler) notAnswerable(account Account, message incomingMessage) string {
	switch {
	case message.Kind == KindTap:
		// A tap is an answer, not a question. The patient has already been
		// told what the reminder said and has pressed one of the two buttons
		// under it; an assistant replying to that is a second conversation
		// nobody asked for, and one that would have to describe a
		// cancellation it did not make. The row is stored, so the model sees
		// the tap in the history next time the patient does write.
		return "a tap is an answer, not a question"

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
func (h *handler) applyTap(ctx context.Context, userID, conversationID string, message incomingMessage, now string) (string, error) {
	// A Messenger or Instagram receipt names its appointment in the
	// button's payload, because those channels do not say which message a
	// quick reply answers.
	if named := tappedAppointment(message.TapChoices); named != "" {
		return h.applyNamedTap(ctx, userID, conversationID, named, message, now)
	}

	if message.RepliesTo == "" {
		h.deps.Logger.Warn("a tap arrived naming no message it answers; no appointment was changed",
			"message_id", message.MessageID, "content", message.Content)

		return "", nil
	}

	appointmentID, err := h.deps.Store.AppointmentForSentMessage(ctx, userID, message.RepliesTo)
	if err != nil {
		return "", fmt.Errorf("look up the reminder %s a tap answers: %w", message.RepliesTo, err)
	}
	if appointmentID == "" {
		h.deps.Logger.Warn("a tap answers a message this clinic never sent about an appointment; no appointment was changed",
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

// applyNamedTap moves the appointment a tap's payload names, once it is
// established that the appointment came out of the very conversation the tap
// arrived in. A payload naming any other appointment changes nothing.
func (h *handler) applyNamedTap(ctx context.Context, userID, conversationID, appointmentID string, message incomingMessage, now string) (string, error) {
	ours, err := h.deps.Store.AppointmentInConversation(ctx, userID, conversationID, appointmentID)
	if err != nil {
		return "", fmt.Errorf("check the appointment %s a tap names: %w", appointmentID, err)
	}
	if !ours {
		h.deps.Logger.Warn("a tap named an appointment that did not come from this conversation; no appointment was changed",
			"message_id", message.MessageID, "appointment_id", appointmentID)

		return "", nil
	}

	action, ok := resolveTap(message.TapChoices)
	if !ok {
		return "", nil
	}

	if err := h.deps.Store.SetAppointmentStatus(ctx, appointmentID, action.Status(), appointments.Patient, now); err != nil {
		return "", fmt.Errorf("record the patient's tap against appointment %s: %w", appointmentID, err)
	}

	h.deps.Logger.Info("a patient's tap moved an appointment",
		"message_id", message.MessageID, "appointment_id", appointmentID, "status", string(action.Status()))

	return appointmentID, nil
}

// createdAppointment is an appointment this delivery itself wrote.
//
// It is what the receipt may be sent about and nothing else is: a write
// refused by the duplicate rule belongs to another delivery, and that is
// the one that speaks for it. Two deliveries both sending a receipt is the
// duplicate message this whole feature exists to avoid.
type createdAppointment struct {
	ID string
	Appointment
}

// checkWithThePatient sends the receipt for an appointment the assistant
// has just written, unless it is owed none.
//
// Nothing here is returned. The appointment is written and the message that
// produced it is stored, which are the two things that must not be lost; a
// receipt that cannot be sent is a message the patient does not get, and
// the reminder still arrives.
func (h *handler) checkWithThePatient(
	ctx context.Context,
	account Account,
	message incomingMessage,
	conversationID string,
	created createdAppointment,
) {
	// Where the receipt goes: the number on the appointment for WhatsApp,
	// and on the other channels the customer the conversation is with —
	// the appointment's phone, if it has one, is for the WhatsApp reminder.
	to := created.ClientPhone
	if message.Channel.ReachedThroughPage() {
		to = message.CustomerPhone
	}

	if reason := h.suppressionReason(account, message.Channel, to, created.Appointment); reason != "" {
		h.deps.Logger.Info("no creation receipt: "+reason, "appointment_id", created.ID)

		return
	}

	if err := h.sendReceipt(ctx, account, message, conversationID, to, created); err != nil {
		h.deps.Logger.Error("the creation receipt failed; the appointment is written and kept",
			"appointment_id", created.ID, "error", err)
	}
}

// recordAppointment reads the conversation and writes the appointment it
// agreed on, if it agreed one and has not already produced one, and returns
// what it wrote.
//
// A nil return is every case where this delivery wrote nothing: no
// agreement, one already written from this conversation, or the duplicate
// rule refusing the write to another delivery that got there first.
//
// The writer is a parameter, narrowed to AppointmentWriter, rather than the
// Store the handler already holds. That is the whole of the guarantee that
// the assistant never moves or deletes an appointment: this function is
// handed no method that could, so no reading of any conversation can reach
// one.
func (h *handler) recordAppointment(
	ctx context.Context,
	writer AppointmentWriter,
	account Account,
	channel Channel,
	conversationID string,
) (*createdAppointment, error) {
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
	already, err := writer.HasAppointmentFromConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("look for an appointment already written from this conversation: %w", err)
	}
	if already {
		return nil, nil
	}

	stored, err := h.deps.Store.ConversationHistory(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("read the conversation history: %w", err)
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
		return nil, fmt.Errorf("read the conversation for an appointment: %w", err)
	}
	if extracted == nil {
		// Most conversations are a question about the opening hours. None of
		// them may leave an appointment behind.
		return nil, nil
	}

	// The customer's number, taken from the conversation rather than from
	// anything the model wrote: it is the one fact here that is not an
	// interpretation.
	//
	// Except where the conversation is not with a number. On Messenger and
	// Instagram the customer is a page-scoped id that no WhatsApp reminder
	// can reach, so the phone is the one the patient wrote down, if they
	// did, and the name falls back to the channel when there is neither.
	customerPhone := ""
	if channel.ReachedThroughPage() {
		customerPhone = digitsOnly(extracted.ClientPhone)
		if extracted.ClientName == "" && customerPhone == "" {
			extracted.ClientName = channel.anonymousName()
		}
	} else if len(stored) > 0 {
		customerPhone = stored[len(stored)-1].CustomerPhone
	}

	appointment := extracted.Appointment(customerPhone, conversationID, h.deps.Now())
	appointmentID, err := writer.CreateAppointment(ctx, account.UserID, appointment)
	if err != nil {
		return nil, fmt.Errorf("write the appointment: %w", err)
	}
	if appointmentID == "" {
		// The duplicate rule, settled by the database rather than by the
		// check above: another delivery of this conversation wrote the
		// appointment while this one was asking the model about it. The
		// appointment that won is the right one, and the delivery that
		// wrote it is the one that tells the patient.
		h.deps.Logger.Info("another delivery had already written this conversation's appointment",
			"conversation_id", conversationID)

		return nil, nil
	}

	h.deps.Logger.Info("wrote an appointment out of a conversation",
		"conversation_id", conversationID, "appointment_id", appointmentID,
		"status", string(appointment.Status), "starts_at", appointment.StartsAt)

	return &createdAppointment{ID: appointmentID, Appointment: appointment}, nil
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

	text, err := h.deps.GenerateReply(ctx, message.Channel, history, message.Content)
	if err != nil {
		return fmt.Errorf("generate a reply: %w", err)
	}

	replyID, err := h.deps.SendMessage(ctx, message.Channel, sendFrom(account, message), account.AccessToken, message.CustomerPhone, text)
	if err != nil {
		return fmt.Errorf("send the reply: %w", err)
	}

	// A second clock reading: in production the AI and the send take seconds,
	// and the reply did not happen when the message arrived.
	err = h.deps.Store.SaveMessage(ctx, account.UserID, StoredMessage{
		MessageID:      replyID,
		ConversationID: conversationID,
		Channel:        message.Channel,
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

// sendFrom is the id a message to this customer goes out from.
func sendFrom(account Account, message incomingMessage) string {
	if account.SendFrom != "" {
		return account.SendFrom
	}

	return message.AccountKey
}
