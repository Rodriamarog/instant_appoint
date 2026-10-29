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

// Account is the connected WhatsApp number a delivery belongs to.
type Account struct {
	UserID      string
	AccessToken string
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

	// An echo is the business's own message, captured so the dashboard shows
	// the whole conversation. Replying would mean the bot answering the
	// business.
	if message.Direction != Inbound {
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
