package reminderjob

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

	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
	"github.com/Rodriamarog/instant_appoint/internal/whatsapp"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// The two collections this package writes into belong to ticket #6, which
// created them in its own migration. They are named here rather than imported
// from migrations so that the two tickets do not have to land in step.
const (
	messagesCollection      = "whatsapp_messages"
	conversationsCollection = "conversations"
)

// calendarEventsCollection is the appointments table. The migration that
// creates it keeps its name private.
const calendarEventsCollection = "calendar_events"

// The markers that make a row in the shared message log a reminder. Both are
// in the predicate of the unique index the claim relies on, so neither is a
// label: changing either changes what the database enforces.
const (
	reminderMessage = "reminder"
	outbound        = "outbound"
	statusSent      = "sent"
	statusFailed    = "failed"
)

// activeConversation is the status an open conversation carries, and part of
// its unique key.
const activeConversation = "active"

// Store is the PocketBase implementation of reminders.Store.
type Store struct {
	app core.App
}

// NewStore returns a store backed by app.
func NewStore(app core.App) *Store {
	return &Store{app: app}
}

// ActiveSettings returns the reminder configuration of everybody who has
// reminders switched on.
//
// One query, once per run. The scheduler this replaces looked the settings
// row up once per appointment, every minute.
func (s *Store) ActiveSettings(context.Context) ([]reminders.Settings, error) {
	records, err := s.app.FindRecordsByFilter(
		migrations.ReminderSettingsCollection, "is_active = true", "", 0, 0)
	if err != nil {
		return nil, err
	}

	settings := make([]reminders.Settings, 0, len(records))
	for _, record := range records {
		settings = append(settings, reminders.Settings{
			UserID:           record.GetString("user_id"),
			LeadTime:         time.Duration(record.GetInt("timing_minutes")) * time.Minute,
			TemplateName:     record.GetString("template_name"),
			TemplateLanguage: record.GetString("template_language"),
			Timezone:         record.GetString("timezone"),
		})
	}

	return settings, nil
}

// AppointmentsStartingBetween returns every appointment, for everybody,
// starting in [from, to].
//
// start_time is a text column holding what the browser wrote, and SQLite
// compares text byte by byte, so the bounds go through internal/isotime —
// the same fixed-width shape the browser produces. A Go time.RFC3339 bound
// would carry no milliseconds, and 'Z' sorts after '.', so it would silently
// exclude every appointment in its own second.
func (s *Store) AppointmentsStartingBetween(_ context.Context, from, to time.Time) ([]reminders.Appointment, error) {
	records, err := s.app.FindRecordsByFilter(
		calendarEventsCollection,
		"start_time >= {:from} && start_time <= {:to}",
		"start_time", 0, 0,
		dbx.Params{"from": isotime.Format(from), "to": isotime.Format(to)},
	)
	if err != nil {
		return nil, err
	}

	appointments := make([]reminders.Appointment, 0, len(records))
	for _, record := range records {
		start, err := isotime.Parse(record.GetString("start_time"))
		if err != nil {
			// A row whose timestamp is not in the stored shape is already
			// sorting wrongly wherever it appears. Reminding about it would
			// mean guessing at when it is.
			s.app.Logger().Warn("reminders: an appointment has an unreadable start time and is being skipped",
				"event_id", record.Id, "error", err)
			continue
		}

		appointments = append(appointments, reminders.Appointment{
			ID:          record.Id,
			UserID:      record.GetString("user_id"),
			Title:       record.GetString("title"),
			ClientPhone: record.GetString("client_phone"),
			StartTime:   start,
		})
	}

	return appointments, nil
}

// AccountFor returns the person's connected Cloud API number, or nil when
// they have none.
//
// Nobody connected is the ordinary case for a new account, not a failure: the
// run skips those appointments rather than attempting a send that could only
// fail, once a minute, forever.
func (s *Store) AccountFor(_ context.Context, userID string) (*reminders.Account, error) {
	record, err := whatsapp.AccountFor(s.app, userID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil

	case err != nil:
		return nil, err
	}

	return &reminders.Account{
		ID: record.Id,
		// The sending number is phone_number_id. waba_id names the business
		// account, which Meta will not accept as a sender.
		PhoneNumberID: record.GetString("phone_number_id"),
		AccessToken:   record.GetString("access_token"),
	}, nil
}

// ReminderAttempts reports what has already been tried for each appointment.
//
// There is no attempt counter in the schema: a claim that fails becomes a
// failed row and is left there, so the rows are the counter. That survives a
// restart, needs no column of its own, and leaves an audit trail of what was
// tried and when.
func (s *Store) ReminderAttempts(_ context.Context, appointmentIDs []string) (map[string]reminders.Attempt, error) {
	attempts := map[string]reminders.Attempt{}
	if len(appointmentIDs) == 0 {
		return attempts, nil
	}

	ids := make([]any, 0, len(appointmentIDs))
	for _, id := range appointmentIDs {
		ids = append(ids, id)
	}

	var rows []struct {
		EventID string `db:"event_id"`
		Status  string `db:"status"`
		SentAt  string `db:"sent_at"`
	}

	// A plain IN, which PocketBase's filter language has no spelling for —
	// writing one as a chain of ORs would put one bound parameter per
	// appointment into the query text every minute.
	err := s.app.DB().
		Select("event_id", "status", "sent_at").
		From(messagesCollection).
		Where(dbx.HashExp{"message_type": reminderMessage}).
		AndWhere(dbx.In("event_id", ids...)).
		All(&rows)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		attempt := attempts[row.EventID]

		switch row.Status {
		case statusSent:
			attempt.Sent = true

		case statusFailed:
			attempt.Failures++
			if at, err := isotime.Parse(row.SentAt); err == nil && at.After(attempt.LastFailureAt) {
				attempt.LastFailureAt = at
			}
		}

		attempts[row.EventID] = attempt
	}

	return attempts, nil
}

// Claim writes the sent row that reserves this reminder.
//
// The row goes in before Meta is called, and the partial unique index on
// (event_id) where message_type='reminder' and status='sent' is what makes
// that a reservation: a second run writing the same row is refused by the
// database, which is the only place the race is visible. Nothing in Go could
// have seen the other run.
func (s *Store) Claim(_ context.Context, claim reminders.Claim) (string, error) {
	collection, err := s.app.FindCollectionByNameOrId(messagesCollection)
	if err != nil {
		return "", err
	}

	at := isotime.Format(claim.At)

	// Resolved before the claim, so the row is written complete. A claim that
	// is then refused leaves the conversation behind, which costs nothing: it
	// is the conversation the winning run is using, and it is keyed uniquely.
	conversationID, err := s.conversation(claim, at)
	if err != nil {
		return "", fmt.Errorf("resolve the customer's conversation: %w", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", claim.UserID)
	record.Set("to_number", claim.ToNumber)
	record.Set("message_content", claim.Content)
	record.Set("message_type", reminderMessage)
	// Both of these were left empty by the scheduler this replaces, so a
	// reminder belonged to no conversation and had no direction.
	record.Set("direction", outbound)
	record.Set("conversation_id", conversationID)
	record.Set("event_id", claim.AppointmentID)
	record.Set("status", statusSent)
	record.Set("sent_at", at)
	record.Set("whatsapp_account_id", claim.AccountID)

	if err := s.app.Save(record); err != nil {
		if IsUniqueViolation(err) {
			return "", reminders.ErrAlreadyClaimed
		}

		return "", err
	}

	return record.Id, nil
}

// MarkSent records the WhatsApp message id against a claim.
func (s *Store) MarkSent(_ context.Context, claimID, messageID string) error {
	record, err := s.app.FindRecordById(messagesCollection, claimID)
	if err != nil {
		return err
	}

	record.Set("message_id", messageID)

	if err := s.app.Save(record); err != nil {
		if IsUniqueViolation(err) {
			// message_id is unique where it is non-empty, so this means Meta
			// handed back an id this app has already stored. The reminder
			// did go out; losing the id is better than failing the run and
			// leaving the row looking unfinished.
			s.app.Logger().Warn("reminders: the message id Meta returned is already stored, leaving it off this row",
				"claim_id", claimID, "message_id", messageID)

			return nil
		}

		return err
	}

	return nil
}

// MarkFailed turns a claim into a failed attempt.
//
// The unique index is predicated on status='sent', so this is what releases
// the reservation and lets a later run try again — and the row it leaves is
// what counts the attempt, so the retry is bounded rather than endless.
func (s *Store) MarkFailed(_ context.Context, claimID string) error {
	record, err := s.app.FindRecordById(messagesCollection, claimID)
	if err != nil {
		return err
	}

	record.Set("status", statusFailed)

	return s.app.Save(record)
}

// conversation returns the id of the customer's active conversation,
// creating it when there is none.
func (s *Store) conversation(claim reminders.Claim, at string) (string, error) {
	record, err := s.findConversation(claim.UserID, claim.ToNumber)
	switch {
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return "", err

	case err == nil:
		return s.touch(record, claim.PhoneNumberID, at)
	}

	collection, err := s.app.FindCollectionByNameOrId(conversationsCollection)
	if err != nil {
		return "", err
	}

	created := core.NewRecord(collection)
	created.Set("user_id", claim.UserID)
	created.Set("customer_phone", claim.ToNumber)
	created.Set("status", activeConversation)
	created.Set("last_message_at", at)
	created.Set("phone_number_id", claim.PhoneNumberID)

	err = s.app.Save(created)
	switch {
	case err == nil:
		return created.Id, nil

	case !IsUniqueViolation(err):
		return "", err
	}

	// Another writer — an inbound message, or a concurrent run — opened the
	// conversation between the lookup and this write. The right answer is the
	// one that won.
	record, err = s.findConversation(claim.UserID, claim.ToNumber)
	if err != nil {
		return "", fmt.Errorf("the conversation was created by something else but could not be read back: %w", err)
	}

	return s.touch(record, claim.PhoneNumberID, at)
}

// touch records the reminder as the conversation's latest activity and the
// number it went out from, so a human replying from the dashboard replies
// from the same number.
func (s *Store) touch(record *core.Record, phoneNumberID, at string) (string, error) {
	record.Set("last_message_at", at)
	record.Set("phone_number_id", phoneNumberID)

	if err := s.app.Save(record); err != nil {
		return "", err
	}

	return record.Id, nil
}

func (s *Store) findConversation(userID, customerPhone string) (*core.Record, error) {
	return s.app.FindFirstRecordByFilter(
		conversationsCollection,
		"user_id = {:userID} && customer_phone = {:customerPhone} && status = {:status}",
		dbx.Params{"userID": userID, "customerPhone": customerPhone, "status": activeConversation},
	)
}

// IsUniqueViolation reports whether err is a unique or primary-key constraint
// failure.
//
// It has to cover two shapes, and the one the production path produces is not
// the obvious one:
//
//  1. app.Save(record) — PocketBase passes the driver error through
//     validators.NormalizeUniqueIndexError, which builds a fresh ozzo
//     validation error with code "validation_not_unique" and discards the
//     original. errors.As against the SQLite type is therefore false, and a
//     detector written only that way would never fire on this path at all.
//     Its silent failure mode is the worst one available: a reminder another
//     run already claimed would surface as a hard error rather than a skip.
//
//  2. raw app.DB() writes — a real *sqlite.Error, code 2067 (unique) or 1555
//     (primary key).
//
// The string check last is defensive: NormalizeUniqueIndexError only rewrites
// an error when it can match a field name in SQLite's message, so an index
// over something that is not a collection field falls through as the raw
// driver message.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var driverError *sqlite.Error
	if errors.As(err, &driverError) {
		switch driverError.Code() {
		case sqlitelib.SQLITE_CONSTRAINT_UNIQUE, sqlitelib.SQLITE_CONSTRAINT_PRIMARYKEY:
			return true
		}
	}

	if isNotUniqueValidation(err) {
		return true
	}

	message := strings.ToLower(err.Error())

	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "value must be unique")
}

// isNotUniqueValidation walks an ozzo validation error, which is a map of one
// error per offending field.
func isNotUniqueValidation(err error) bool {
	switch typed := err.(type) {
	case validation.Errors:
		for _, inner := range typed {
			if isNotUniqueValidation(inner) {
				return true
			}
		}

	case validation.ErrorObject:
		return typed.Code() == "validation_not_unique"

	case validation.Error:
		return typed.Code() == "validation_not_unique"
	}

	return false
}
