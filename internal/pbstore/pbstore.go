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
	"time"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	sqlite "modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// activeConversation is the status an open conversation carries. It is part of
// the conversation's unique key, so closing one frees the customer for a new
// one.
const activeConversation = "active"

// conversationMessage is the message_type an ordinary message in a
// conversation carries, as opposed to "reminder" or a tap on a button.
const conversationMessage = "text"

// reminderMessage is the message_type internal/reminderjob writes on the row
// recording a reminder it sent. That row is the only thing holding both
// Meta's id for the reminder and the appointment it was about, which is what
// a tap on the reminder's buttons is matched through.
//
// Spelled here as well as there because neither package may import the
// other, and it sits in the predicate of a unique index besides: it is not a
// label, and a disagreement between the two spellings would quietly match no
// tap to anything.
const reminderMessage = "reminder"

// receiptMessage is the same row for the receipt the webhook sends the
// moment the assistant writes an appointment. It asks the same question
// with the same two buttons, so a tap on it is matched the same way.
//
// Taken from internal/webhook rather than spelled again, because this
// package writes that row too — SaveMessage puts the kind in message_type —
// so there is one spelling and the lookup cannot drift from the write.
const receiptMessage = string(webhook.KindReceipt)

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

	userID := record.GetString("user_id")

	account := &webhook.Account{
		UserID:      userID,
		AccessToken: record.GetString("access_token"),

		// What a clinic with no settings row looks like: the assistant off,
		// no reminders, and the zone every other reader of a missing
		// setting falls back to. A zone has to be something — a receipt
		// naming a UTC hour sends somebody in at ten at night — and this is
		// the same value the migration gives a row it creates.
		Mode:     webhook.ModeOff,
		Timezone: migrations.DefaultTimezone,
	}

	if err := s.readSettings(userID, account); err != nil {
		return nil, err
	}

	return account, nil
}

// readSettings fills in what the clinic behind userID has configured: how
// much the assistant may do, and what its reminders do, which is what
// decides whether a creation receipt is worth sending.
//
// It is a second query rather than columns on the account because these
// belong to the business, not to the number: a clinic with two connected
// numbers answers on both or neither. One query for all of it, though —
// they are read together on every inbound message, and the row is already
// in hand once it has been found.
//
// No settings row leaves the defaults the caller set, and is not an error.
// A clinic that has connected a number but never opened the settings is the
// state every newly registered one is in, and the assistant stays quiet
// for it.
//
// A real database failure is returned rather than quietly read as off,
// because the handler turns an error into a 500 and the router redelivers.
// Reading a fault as off would silence every clinic's assistant for as long
// as it lasted, with nothing anywhere saying why.
func (s *WebhookStore) readSettings(userID string, account *webhook.Account) error {
	record, err := s.app.FindFirstRecordByFilter(
		migrations.BusinessSettingsCollection,
		"user_id = {:userID}",
		dbx.Params{"userID": userID},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil

	case err != nil:
		return err
	}

	account.Mode = webhook.Mode(record.GetString(migrations.AIModeField))

	// Both halves, because either one missing means internal/reminders
	// sends nothing: it skips a clinic that is not active and counts one
	// with no template as NoTemplate. A receipt held back in favour of a
	// reminder that is never sent leaves the patient with no message at
	// all.
	account.SendsReminders = record.GetBool("is_active") && record.GetString("template_name") != ""
	account.ReminderLeadTime = time.Duration(record.GetInt("timing_minutes")) * time.Minute

	// An empty zone would make time.LoadLocation return UTC, silently,
	// which is the one wrong answer that looks like an answer.
	if zone := record.GetString("timezone"); zone != "" {
		account.Timezone = zone
	}

	return nil
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

	messageType := conversationMessage
	if message.Kind != webhook.KindText {
		messageType = string(message.Kind)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	// to_number holds the customer in both directions; the name is from when
	// this collection only recorded reminders going out.
	record.Set("to_number", message.CustomerPhone)
	record.Set("message_content", message.Content)
	record.Set("message_type", messageType)
	record.Set("direction", string(message.Direction))
	record.Set("conversation_id", message.ConversationID)
	record.Set("sent_at", message.SentAt)
	record.Set("message_id", message.MessageID)
	// Empty on everything but a tap that moved an appointment. The reminder
	// index this column also serves is predicated on message_type =
	// 'reminder', so a tap naming the same appointment does not collide with
	// the reminder it answers.
	record.Set("event_id", message.AppointmentID)

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

// HasAppointmentFromConversation reports whether an appointment already
// records this conversation as where it came from.
//
// An empty conversation id answers false without asking the database. Every
// appointment booked at the desk carries one, so the query would match the
// first of those and report that the conversation had already produced an
// appointment — after which the assistant would never write another for
// anybody.
func (s *WebhookStore) HasAppointmentFromConversation(_ context.Context, conversationID string) (bool, error) {
	if conversationID == "" {
		return false, nil
	}

	_, err := s.app.FindFirstRecordByFilter(
		migrations.AppointmentsCollection,
		"conversation_id = {:conversationID}",
		dbx.Params{"conversationID": conversationID},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil

	case err != nil:
		return false, err
	}

	return true, nil
}

// CreateAppointment writes a new appointment for a clinic.
//
// There is no sibling here that removes one, and the one that changes a
// status is not reachable from the step that reads conversations — see
// webhook.AppointmentWriter, which is the interface that step is handed.
//
// A second appointment from the same conversation is a no-op rather than an
// error. The handler checks before it writes, but the check and the write
// cannot be one operation in Go, and two deliveries of one conversation can
// be in flight on two servers at once. The unique index is the only thing
// that can see the other one, and the right answer to losing that race is
// the appointment that won — not a failed delivery, which would have the
// router redeliver a message that is already stored.
//
// The returned id is empty in exactly that case, and non-empty whenever a
// row was written. It is what tells the handler whether the appointment it
// is about to tell the patient about is one it created.
func (s *WebhookStore) CreateAppointment(_ context.Context, userID string, appointment webhook.Appointment) (string, error) {
	collection, err := s.app.FindCollectionByNameOrId(migrations.AppointmentsCollection)
	if err != nil {
		return "", err
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("client_name", appointment.ClientName)
	record.Set("client_phone", appointment.ClientPhone)
	record.Set("start_time", appointment.StartsAt)
	record.Set("end_time", appointment.EndsAt)
	record.Set("notes", appointment.Notes)
	record.Set(migrations.StatusField, string(appointment.Status))
	record.Set("created_by", string(appointment.CreatedBy))
	record.Set(migrations.StatusSetByField, string(appointment.StatusSetBy))
	record.Set(migrations.StatusSetAtField, appointment.StatusSetAt)
	record.Set("conversation_id", appointment.ConversationID)
	record.Set(migrations.LanguageField, appointment.Language)

	if err := s.app.Save(record); err != nil {
		if appointment.ConversationID != "" && IsUniqueViolation(err) {
			// No id, because no row of this app's was written. The caller
			// sends the patient a receipt about what it created, and the
			// appointment that won this race is not its to speak for.
			return "", nil
		}

		return "", err
	}

	return record.Id, nil
}

// AppointmentForSentMessage returns the appointment a message this app sent
// was about, given Meta's id for that message, or "" when this clinic sent
// no such message.
//
// Matched on that message's own wamid rather than on the customer's number,
// because the number does not identify an appointment: a patient with two
// bookings in one week has two of these out at once, and the only thing
// separating them is which message the tap answers.
//
// Scoped to the clinic, and to the two kinds of message that ask a patient
// about one appointment: the reminder, and the receipt sent the moment the
// assistant writes one. A tap can answer anything the business ever sent;
// one answering an ordinary reply finds nothing here, which is the correct
// answer and not a fault.
func (s *WebhookStore) AppointmentForSentMessage(_ context.Context, userID, sentMessageID string) (string, error) {
	if sentMessageID == "" {
		return "", nil
	}

	record, err := s.app.FindFirstRecordByFilter(
		migrations.WhatsAppMessagesCollection,
		"user_id = {:userID} && message_id = {:messageID} && (message_type = {:reminder} || message_type = {:receipt})",
		dbx.Params{
			"userID":    userID,
			"messageID": sentMessageID,
			"reminder":  reminderMessage,
			"receipt":   receiptMessage,
		},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil

	case err != nil:
		return "", err
	}

	return record.GetString("event_id"), nil
}

// SetAppointmentStatus moves an appointment and records who moved it and
// when.
//
// Three columns and nothing else. The appointment's own details are
// somebody's work — a phone number corrected while this was in flight, a
// note typed at the desk — and a status change is not an edit of them.
func (s *WebhookStore) SetAppointmentStatus(
	_ context.Context,
	appointmentID string,
	status appointments.Status,
	by appointments.Actor,
	at string,
) error {
	record, err := s.app.FindRecordById(migrations.AppointmentsCollection, appointmentID)
	if err != nil {
		return err
	}

	record.Set(migrations.StatusField, string(status))
	record.Set(migrations.StatusSetByField, string(by))
	record.Set(migrations.StatusSetAtField, at)

	return s.app.Save(record)
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
