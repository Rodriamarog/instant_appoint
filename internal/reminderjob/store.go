package reminderjob

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
	"github.com/Rodriamarog/instant_appoint/internal/whatsapp"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// The markers that make a row in the shared message log a reminder. Both are
// in the predicate of the unique index the claim relies on, so neither is a
// label: changing either changes what the database enforces.
const (
	reminderMessage = "reminder"
	outbound        = "outbound"
	statusSent      = "sent"
	statusFailed    = "failed"
)

// Store is the PocketBase implementation of reminders.Store.
type Store struct {
	app core.App

	// conversations is the webhook's store, reused for its find-or-create.
	// A reminder and an inbound message open the same customer's thread the
	// same way, and both have to survive losing the race to the other: that
	// is one piece of reasoning, settled by one unique index, and it should
	// not exist twice.
	conversations *pbstore.WebhookStore
}

// NewStore returns a store backed by app.
func NewStore(app core.App) *Store {
	return &Store{app: app, conversations: pbstore.NewWebhookStore(app)}
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
		migrations.CalendarEventsCollection,
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
		EventID   string `db:"event_id"`
		Status    string `db:"status"`
		SentAt    string `db:"sent_at"`
		MessageID string `db:"message_id"`
	}

	// A plain IN, which PocketBase's filter language has no spelling for —
	// writing one as a chain of ORs would put one bound parameter per
	// appointment into the query text every minute.
	err := s.app.DB().
		Select("event_id", "status", "sent_at", "message_id").
		From(migrations.WhatsAppMessagesCollection).
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

			// A sent row with no message id is a claim whose send never
			// finished reporting back — the process died between writing the
			// claim and recording what Meta said. reminders.Attempt.Abandoned
			// decides, from how old it is, whether that is still in flight.
			if row.MessageID == "" {
				attempt.Incomplete = true
				if at, err := isotime.Parse(row.SentAt); err == nil {
					attempt.ClaimedAt = at
				} else {
					// Unreadable, so unaged. Treating it as claimed right now
					// keeps it suppressed rather than resending every minute.
					attempt.ClaimedAt = time.Now()
				}
			}

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
//
// A refusal is not automatically another live run, though. The row holding the
// reservation may have been left by a process that died mid-send, in which
// case it says sent, carries no message id, and would suppress this reminder
// for good. Once it is old enough to be certainly dead
// (reminders.ReclaimAfter) it is turned into a failed attempt — which releases
// the index, leaves the crash in the audit trail and counts towards the retry
// bound — and the claim is written again.
func (s *Store) Claim(ctx context.Context, claim reminders.Claim) (string, error) {
	collection, err := s.app.FindCollectionByNameOrId(migrations.WhatsAppMessagesCollection)
	if err != nil {
		return "", err
	}

	at := isotime.Format(claim.At)

	// Resolved before the claim, so the row is written complete. A claim that
	// is then refused leaves the conversation behind, which costs nothing: it
	// is the conversation the winning run is using, and it is keyed uniquely.
	conversationID, err := s.conversations.UpsertConversation(
		ctx, claim.UserID, claim.ToNumber, claim.PhoneNumberID, at)
	if err != nil {
		return "", fmt.Errorf("resolve the customer's conversation: %w", err)
	}

	write := func() *core.Record {
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

		return record
	}

	record := write()
	err = s.app.Save(record)
	switch {
	case err == nil:
		return record.Id, nil

	case !pbstore.IsUniqueViolation(err):
		return "", err
	}

	released, releaseErr := s.releaseAbandonedClaim(claim.AppointmentID, claim.At)
	if releaseErr != nil {
		return "", fmt.Errorf("check whether the existing claim was abandoned: %w", releaseErr)
	}
	if !released {
		return "", reminders.ErrAlreadyClaimed
	}

	// Two runs can reach this together, both having seen the same dead row.
	// The index settles that the same way it settles every other race: one of
	// them is refused, and steps away.
	record = write()
	if err := s.app.Save(record); err != nil {
		if pbstore.IsUniqueViolation(err) {
			return "", reminders.ErrAlreadyClaimed
		}

		return "", err
	}

	return record.Id, nil
}

// releaseAbandonedClaim turns a claim nothing is finishing into a failed
// attempt, and says whether it did.
//
// Only a row that is certainly dead is touched: status='sent' with no message
// id, written at least reminders.ReclaimAfter ago. A claim a live run is still
// working through is left exactly where it is.
func (s *Store) releaseAbandonedClaim(appointmentID string, now time.Time) (bool, error) {
	record, err := s.app.FindFirstRecordByFilter(
		migrations.WhatsAppMessagesCollection,
		"event_id = {:eventID} && message_type = {:type} && status = {:status} && message_id = {:empty}",
		dbx.Params{
			"eventID": appointmentID,
			"type":    reminderMessage,
			"status":  statusSent,
			"empty":   "",
		},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Either nothing holds the reminder any more, or what holds it has a
		// message id: a reminder that really did go out.
		return false, nil

	case err != nil:
		return false, err
	}

	claimedAt, err := isotime.Parse(record.GetString("sent_at"))
	if err != nil {
		// Its age is unknowable, so it cannot be declared dead. Resending on
		// a guess would mean resending every minute.
		s.app.Logger().Warn("reminders: an unfinished claim has an unreadable timestamp and is being left alone",
			"claim_id", record.Id, "event_id", appointmentID, "error", err)

		return false, nil
	}

	if !(reminders.Attempt{Sent: true, Incomplete: true, ClaimedAt: claimedAt}).Abandoned(now) {
		return false, nil
	}

	record.Set("status", statusFailed)
	if err := s.app.Save(record); err != nil {
		return false, err
	}

	s.app.Logger().Warn("reminders: a claim was left unfinished and is being retried — the send may have reached Meta before the process stopped, so the customer may see it twice",
		"claim_id", record.Id, "event_id", appointmentID, "claimed_at", record.GetString("sent_at"))

	return true, nil
}

// MarkSent records the WhatsApp message id against a claim.
func (s *Store) MarkSent(_ context.Context, claimID, messageID string) error {
	record, err := s.app.FindRecordById(migrations.WhatsAppMessagesCollection, claimID)
	if err != nil {
		return err
	}

	record.Set("message_id", messageID)

	if err := s.app.Save(record); err != nil {
		if pbstore.IsUniqueViolation(err) {
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
	record, err := s.app.FindRecordById(migrations.WhatsAppMessagesCollection, claimID)
	if err != nil {
		return err
	}

	record.Set("status", statusFailed)

	return s.app.Save(record)
}
