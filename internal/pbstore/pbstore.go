// Package pbstore is the PocketBase half of the webhook.
//
// internal/webhook is written against interfaces and imports nothing from
// PocketBase, so that its behaviour — which is a long list of cases Meta can
// produce — is testable with no database and no network. Everything that
// knows about collections, records and SQLite lives here, on the other side of
// those interfaces.
//
// The two races this app has to win are both settled here, and both are
// settled by a unique index rather than by a check followed by a write: two
// deliveries of one message, and two messages opening one customer's
// conversation at the same moment. Go cannot see the other delivery; the
// database can.
package pbstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	sqlite "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"

	"github.com/Rodriamarog/instant_appoint/internal/webhook"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// activeConversation is the status an open conversation carries. It is part of
// the conversation's unique key, so closing one frees the customer for a new
// one.
const activeConversation = "active"

// conversationMessage is the message_type every message in a conversation
// carries, as opposed to "reminder".
const conversationMessage = "text"

// WebhookStore persists what the webhook handler receives.
type WebhookStore struct {
	app core.App
}

// NewWebhookStore returns a store backed by app.
func NewWebhookStore(app core.App) *WebhookStore {
	return &WebhookStore{app: app}
}

// FindAccount returns the connected account for a phone number id, or nil when
// no account has that number.
//
// This is deliberately looser than every other account lookup in this app,
// which also require account_type and is_active: a message that arrived is a
// message that has to be stored. A number mid-reconnect, or one still carrying
// the retired QR flow's account_type, would otherwise have its customers'
// messages dropped. The tightening belongs on sending, not on receiving.
func (s *WebhookStore) FindAccount(_ context.Context, phoneNumberID string) (*webhook.Account, error) {
	record, err := s.app.FindFirstRecordByFilter(
		migrations.WhatsAppAccountsCollection,
		"phone_number_id = {:phoneNumberID}",
		dbx.Params{"phoneNumberID": phoneNumberID},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Not an error. The router forwards every number it serves, and most
		// of them belong to other apps behind it.
		return nil, nil

	case err != nil:
		return nil, err
	}

	return &webhook.Account{
		UserID:      record.GetString("user_id"),
		AccessToken: record.GetString("access_token"),
	}, nil
}

// HasMessage reports whether a message with this WhatsApp id is already
// stored.
func (s *WebhookStore) HasMessage(_ context.Context, messageID string) (bool, error) {
	_, err := s.app.FindFirstRecordByFilter(
		migrations.WhatsAppMessagesCollection,
		"message_id = {:messageID}",
		dbx.Params{"messageID": messageID},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil

	case err != nil:
		return false, err
	}

	return true, nil
}

// UpsertConversation returns the id of the customer's active conversation,
// creating it if there is none, and records at as the time of its latest
// message.
//
// phoneNumberID is written on both paths, so a conversation follows the number
// the customer is actually talking to and a human replying from the dashboard
// is sent from it.
func (s *WebhookStore) UpsertConversation(_ context.Context, userID, customerPhone, phoneNumberID, at string) (string, error) {
	record, err := s.findConversation(userID, customerPhone)
	switch {
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return "", err

	case err == nil:
		record.Set("last_message_at", at)
		record.Set("phone_number_id", phoneNumberID)
		if err := s.app.Save(record); err != nil {
			return "", err
		}

		return record.Id, nil
	}

	collection, err := s.app.FindCollectionByNameOrId(migrations.ConversationsCollection)
	if err != nil {
		return "", err
	}

	created := core.NewRecord(collection)
	created.Set("user_id", userID)
	created.Set("customer_phone", customerPhone)
	created.Set("status", activeConversation)
	created.Set("last_message_at", at)
	created.Set("phone_number_id", phoneNumberID)

	err = s.app.Save(created)
	switch {
	case err == nil:
		return created.Id, nil

	case !IsUniqueViolation(err):
		return "", err
	}

	// Another delivery for this customer created the conversation between the
	// lookup above and this write. The unique index is what made that
	// visible — nothing in this process could have seen it — and the right
	// answer is the conversation that won, not a failed delivery.
	record, err = s.findConversation(userID, customerPhone)
	if err != nil {
		return "", fmt.Errorf("another delivery created the conversation but it could not be read back: %w", err)
	}

	record.Set("last_message_at", at)
	record.Set("phone_number_id", phoneNumberID)
	if err := s.app.Save(record); err != nil {
		return "", err
	}

	return record.Id, nil
}

func (s *WebhookStore) findConversation(userID, customerPhone string) (*core.Record, error) {
	return s.app.FindFirstRecordByFilter(
		migrations.ConversationsCollection,
		"user_id = {:userID} && customer_phone = {:customerPhone} && status = {:status}",
		dbx.Params{"userID": userID, "customerPhone": customerPhone, "status": activeConversation},
	)
}

// SaveMessage stores one message.
//
// A message this store already has is a no-op rather than an error. The
// handler's in-flight guard is per process, so a redelivery landing on a
// second server or after a restart reaches here with an id already stored, and
// failing would answer the router non-2xx and have it deliver again forever.
//
// Messages with no WhatsApp id are all stored: the unique index is predicated
// on a non-empty id, so they neither collide with each other nor get mistaken
// for duplicates.
func (s *WebhookStore) SaveMessage(_ context.Context, userID string, message webhook.StoredMessage) error {
	collection, err := s.app.FindCollectionByNameOrId(migrations.WhatsAppMessagesCollection)
	if err != nil {
		return err
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	// to_number holds the customer in both directions; the name is from when
	// this collection only recorded reminders going out.
	record.Set("to_number", message.CustomerPhone)
	record.Set("message_content", message.Content)
	record.Set("message_type", conversationMessage)
	record.Set("direction", string(message.Direction))
	record.Set("conversation_id", message.ConversationID)
	record.Set("sent_at", message.SentAt)
	record.Set("message_id", message.MessageID)

	if err := s.app.Save(record); err != nil {
		if message.MessageID != "" && IsUniqueViolation(err) {
			return nil
		}

		return err
	}

	return nil
}

// ConversationHistory returns every message of a conversation, oldest first.
//
// sent_at is text holding a fixed-width ISO-8601 timestamp, so this sort is a
// byte comparison that happens to be chronological — see internal/isotime for
// why every writer has to keep producing that exact shape.
func (s *WebhookStore) ConversationHistory(_ context.Context, conversationID string) ([]webhook.StoredMessage, error) {
	// No limit: a conversation is handed to the model whole, which is what the
	// TypeScript did. A long-running thread will eventually need a window, and
	// that is a change to make deliberately rather than by accident here.
	records, err := s.app.FindRecordsByFilter(
		migrations.WhatsAppMessagesCollection,
		"conversation_id = {:conversationID}",
		"sent_at",
		0,
		0,
		dbx.Params{"conversationID": conversationID},
	)
	if err != nil {
		return nil, err
	}

	history := make([]webhook.StoredMessage, 0, len(records))
	for _, record := range records {
		history = append(history, webhook.StoredMessage{
			MessageID:      record.GetString("message_id"),
			ConversationID: record.GetString("conversation_id"),
			CustomerPhone:  record.GetString("to_number"),
			Content:        record.GetString("message_content"),
			Direction:      webhook.Direction(record.GetString("direction")),
			SentAt:         record.GetString("sent_at"),
		})
	}

	return history, nil
}

// IsUniqueViolation reports whether err is a unique or primary-key constraint
// failure.
//
// It has to cover two unrelated error shapes, and the obvious implementation
// covers the wrong one:
//
//   - app.Save passes the driver's error through PocketBase's
//     NormalizeUniqueIndexError, which builds a fresh ozzo validation error
//     with code "validation_not_unique" and discards the original. errors.As
//     against the SQLite error type returns false here — on exactly the path
//     this app writes through.
//   - A raw app.DB() write returns the driver's own *sqlite.Error, code 2067.
//
// An errors.As-only predicate would therefore never fire where it matters, and
// every redelivered message would turn a no-op into a failed delivery that the
// router then retries. Both shapes were reproduced against v0.40.6 and both
// are covered by tests.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var driverErr *sqlite.Error
	if errors.As(err, &driverErr) {
		switch driverErr.Code() {
		case sqlitelib.SQLITE_CONSTRAINT_UNIQUE, sqlitelib.SQLITE_CONSTRAINT_PRIMARYKEY:
			return true
		}
	}

	if isNotUniqueValidation(err) {
		return true
	}

	// Defensive, for shapes not enumerated above: the normaliser only rewrites
	// the error when it can match a collection field name in the SQLite
	// message, so an index over something that is not a field falls through as
	// the raw driver error string.
	message := strings.ToLower(err.Error())

	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "value must be unique")
}

// isNotUniqueValidation walks an ozzo validation error, which is a map of one
// error per offending field and so has to be descended into rather than
// compared.
func isNotUniqueValidation(err error) bool {
	var errs validation.Errors
	if errors.As(err, &errs) {
		for _, inner := range errs {
			if isNotUniqueValidation(inner) {
				return true
			}
		}

		return false
	}

	var object validation.ErrorObject
	if errors.As(err, &object) {
		return object.Code() == "validation_not_unique"
	}

	var wrapped validation.Error
	if errors.As(err, &wrapped) {
		return wrapped.Code() == "validation_not_unique"
	}

	return false
}
