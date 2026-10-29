package reminderjob_test

import (
	"context"
	"errors"
	"testing"
	"time"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	sqlite "modernc.org/sqlite"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
	"github.com/Rodriamarog/instant_appoint/internal/reminderjob"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

func TestActiveSettingsReadsOnlyThePeopleWhoHaveRemindersSwitchedOn(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	bob := newUser(t, app, "bob@instantappoint.test")

	save(t, app, "reminder_settings", map[string]any{
		"user_id": alice, "timing_minutes": 90, "is_active": true,
		"template_name": "appointment_reminder", "template_language": "es", "timezone": "America/Tijuana",
	})
	save(t, app, "reminder_settings", map[string]any{
		"user_id": bob, "timing_minutes": 30, "is_active": false,
		"template_name": "appointment_reminder", "template_language": "es", "timezone": "America/Tijuana",
	})

	active, err := reminderjob.NewStore(app).ActiveSettings(context.Background())
	if err != nil {
		t.Fatalf("ActiveSettings: %v", err)
	}

	if len(active) != 1 {
		t.Fatalf("ActiveSettings returned %d rows, want only the one with reminders on: %+v", len(active), active)
	}
	settings := active[0]
	if settings.UserID != alice {
		t.Errorf("UserID = %q, want %q", settings.UserID, alice)
	}
	if settings.LeadTime != 90*time.Minute {
		t.Errorf("LeadTime = %s, want 90m — timing_minutes is stored in minutes", settings.LeadTime)
	}
	if settings.TemplateName != "appointment_reminder" || settings.TemplateLanguage != "es" {
		t.Errorf("template = %q/%q, want appointment_reminder/es", settings.TemplateName, settings.TemplateLanguage)
	}
	if settings.Timezone != "America/Tijuana" {
		t.Errorf("Timezone = %q, want America/Tijuana", settings.Timezone)
	}
}

// start_time is a text column compared byte by byte, so the window bounds
// have to be written in the same fixed-width shape the browser stores. A
// bound without milliseconds would sort after every millisecond-bearing value
// in its second and silently drop appointments from the window.
func TestTheWindowBoundsAreComparedAgainstTimestampsAsStored(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")

	from := time.Date(2026, 11, 3, 16, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)

	appointmentAt := func(id, start string) {
		save(t, app, "calendar_events", map[string]any{
			"user_id": alice, "title": id, "client_phone": "5216641234567",
			"start_time": start, "end_time": start,
		})
	}

	appointmentAt("just before the window", "2026-11-03T15:59:59.999Z")
	appointmentAt("on the near edge", "2026-11-03T16:00:00.000Z")
	appointmentAt("a millisecond inside the near edge", "2026-11-03T16:00:00.001Z")
	appointmentAt("inside", "2026-11-03T17:00:00.500Z")
	appointmentAt("on the far edge", "2026-11-03T18:00:00.000Z")
	appointmentAt("a millisecond past the far edge", "2026-11-03T18:00:00.001Z")

	found, err := reminderjob.NewStore(app).AppointmentsStartingBetween(context.Background(), from, to)
	if err != nil {
		t.Fatalf("AppointmentsStartingBetween: %v", err)
	}

	var titles []string
	for _, appointment := range found {
		titles = append(titles, appointment.Title)
	}

	want := []string{
		"on the near edge",
		"a millisecond inside the near edge",
		"inside",
		"on the far edge",
	}
	if len(titles) != len(want) {
		t.Fatalf("window held %v, want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Errorf("appointment %d = %q, want %q (the window is also sorted by start)", i, titles[i], want[i])
		}
	}
}

func TestAnAppointmentCarriesWhatTheReminderNeeds(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")

	id := save(t, app, "calendar_events", map[string]any{
		"user_id": alice, "title": "Ana Lopez", "client_phone": "5216641234567",
		"start_time": "2026-11-03T16:30:00.000Z", "end_time": "2026-11-03T17:00:00.000Z",
		"notes": "second filling",
	})

	from := time.Date(2026, 11, 3, 16, 0, 0, 0, time.UTC)
	found, err := reminderjob.NewStore(app).AppointmentsStartingBetween(context.Background(), from, from.Add(time.Hour))
	if err != nil {
		t.Fatalf("AppointmentsStartingBetween: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d appointments, want 1", len(found))
	}

	appointment := found[0]
	if appointment.ID != id || appointment.UserID != alice {
		t.Errorf("appointment = %+v, want id %q owned by %q", appointment, id, alice)
	}
	// The event dialog puts the client's name in the title. The scheduler
	// this replaces sent the notes field instead, verbatim.
	if appointment.Title != "Ana Lopez" || appointment.ClientPhone != "5216641234567" {
		t.Errorf("appointment = %+v, want the title as the client name", appointment)
	}
	if want := time.Date(2026, 11, 3, 16, 30, 0, 0, time.UTC); !appointment.StartTime.Equal(want) {
		t.Errorf("StartTime = %s, want %s", appointment.StartTime, want)
	}
}

func TestAccountForIsNothingRatherThanAnErrorWhenNoNumberIsConnected(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	store := reminderjob.NewStore(app)

	account, err := store.AccountFor(context.Background(), alice)
	if err != nil {
		t.Fatalf("AccountFor with no connected number: %v, want no error", err)
	}
	if account != nil {
		t.Fatalf("AccountFor returned %+v, want nothing", account)
	}

	id := connectNumber(t, app, alice)

	account, err = store.AccountFor(context.Background(), alice)
	if err != nil {
		t.Fatalf("AccountFor: %v", err)
	}
	if account == nil {
		t.Fatal("AccountFor returned nothing for a connected number")
	}
	if account.ID != id {
		t.Errorf("account id = %q, want %q", account.ID, id)
	}
	// The sending number is phone_number_id, not the WhatsApp Business
	// account id.
	if account.PhoneNumberID != "200000000000002" {
		t.Errorf("PhoneNumberID = %q, want the number id rather than the WABA id", account.PhoneNumberID)
	}
	if account.AccessToken == "" {
		t.Error("the account carries no access token, so nothing could be sent with it")
	}
}

// A number that has been disconnected is not a number to send from.
func TestAccountForIgnoresAnInactiveNumber(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")

	record, err := app.FindRecordById("whatsapp_accounts", connectNumber(t, app, alice))
	if err != nil {
		t.Fatalf("read the account back: %v", err)
	}
	record.Set("is_active", false)
	if err := app.Save(record); err != nil {
		t.Fatalf("disconnect the account: %v", err)
	}

	account, err := reminderjob.NewStore(app).AccountFor(context.Background(), alice)
	if err != nil {
		t.Fatalf("AccountFor: %v", err)
	}
	if account != nil {
		t.Errorf("AccountFor returned %+v for a disconnected number, want nothing", account)
	}
}

func TestReminderAttemptsSeesWhatHasBeenSentAndCountsWhatHasFailed(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")

	row := func(eventID, status, sentAt string) {
		save(t, app, "whatsapp_messages", map[string]any{
			"user_id": alice, "to_number": "5216641234567", "event_id": eventID,
			"message_type": "reminder", "direction": "outbound", "status": status, "sent_at": sentAt,
		})
	}

	row("evt-sent", "sent", "2026-11-03T16:00:00.000Z")
	row("evt-failed", "failed", "2026-11-03T16:00:00.000Z")
	row("evt-failed", "failed", "2026-11-03T16:10:00.000Z")
	// A chat message about the same appointment is not a reminder attempt.
	save(t, app, "whatsapp_messages", map[string]any{
		"user_id": alice, "to_number": "5216641234567", "event_id": "evt-failed",
		"message_type": "text", "direction": "inbound", "sent_at": "2026-11-03T16:20:00.000Z",
	})

	attempts, err := reminderjob.NewStore(app).ReminderAttempts(context.Background(),
		[]string{"evt-sent", "evt-failed", "evt-untouched"})
	if err != nil {
		t.Fatalf("ReminderAttempts: %v", err)
	}

	if !attempts["evt-sent"].Sent {
		t.Error("the appointment with a sent reminder is not reported as sent")
	}
	failed := attempts["evt-failed"]
	if failed.Sent {
		t.Error("an appointment with only failed attempts is reported as sent")
	}
	if failed.Failures != 2 {
		t.Errorf("Failures = %d, want 2 — the chat message is not an attempt", failed.Failures)
	}
	if want := time.Date(2026, 11, 3, 16, 10, 0, 0, time.UTC); !failed.LastFailureAt.Equal(want) {
		t.Errorf("LastFailureAt = %s, want the most recent failure at %s", failed.LastFailureAt, want)
	}
	if attempt, ok := attempts["evt-untouched"]; ok && (attempt.Sent || attempt.Failures != 0) {
		t.Errorf("an appointment nothing has been tried for reports %+v", attempt)
	}
}

// claimFor is the claim the scheduler would write for one appointment.
func claimFor(eventID, userID, accountID string, at time.Time) reminders.Claim {
	return reminders.Claim{
		AppointmentID: eventID,
		UserID:        userID,
		AccountID:     accountID,
		PhoneNumberID: "200000000000002",
		ToNumber:      "5216641234567",
		Content:       "appointment_reminder (es): Ana Lopez | 03/11/2026 | 08:30",
		At:            at,
	}
}

func TestAClaimWritesOneOutboundReminderAttachedToTheCustomersConversation(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	accountID := connectNumber(t, app, alice)
	at := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)

	claimID, err := reminderjob.NewStore(app).Claim(context.Background(), claimFor("evt1", alice, accountID, at))
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	rows := reminderRows(t, app, "evt1")
	if len(rows) != 1 {
		t.Fatalf("the claim wrote %d rows, want exactly 1", len(rows))
	}
	row := rows[0]
	if row.Id != claimID {
		t.Errorf("Claim returned %q but wrote %q", claimID, row.Id)
	}

	for field, want := range map[string]string{
		"user_id":             alice,
		"to_number":           "5216641234567",
		"message_type":        "reminder",
		"direction":           "outbound",
		"status":              "sent",
		"event_id":            "evt1",
		"whatsapp_account_id": accountID,
		"sent_at":             iso(at),
		"message_content":     "appointment_reminder (es): Ana Lopez | 03/11/2026 | 08:30",
	} {
		if got := row.GetString(field); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}

	// The scheduler this replaces left both of these empty, so a reminder
	// appeared in no conversation and had no direction.
	conversationID := row.GetString("conversation_id")
	if conversationID == "" {
		t.Fatal("the reminder was written with no conversation")
	}

	conversation, err := app.FindRecordById("conversations", conversationID)
	if err != nil {
		t.Fatalf("read the conversation back: %v", err)
	}
	if conversation.GetString("customer_phone") != "5216641234567" || conversation.GetString("user_id") != alice {
		t.Errorf("the reminder is attached to the wrong conversation: %v", conversation.FieldsData())
	}
	if conversation.GetString("phone_number_id") != "200000000000002" {
		t.Error("the conversation does not record which number the reminder went out from")
	}
}

// A customer who already has a conversation gets the reminder in it, rather
// than a second thread.
func TestAClaimReusesTheCustomersExistingConversation(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	accountID := connectNumber(t, app, alice)

	existing := save(t, app, "conversations", map[string]any{
		"user_id": alice, "customer_phone": "5216641234567", "status": "active",
		"last_message_at": "2026-11-01T10:00:00.000Z", "phone_number_id": "200000000000002",
	})

	at := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)
	if _, err := reminderjob.NewStore(app).Claim(context.Background(), claimFor("evt1", alice, accountID, at)); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if got := reminderRows(t, app, "evt1")[0].GetString("conversation_id"); got != existing {
		t.Errorf("the reminder opened conversation %q instead of using %q", got, existing)
	}

	all, err := app.FindRecordsByFilter("conversations", "user_id != ''", "", 0, 0)
	if err != nil {
		t.Fatalf("count the conversations: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("there are now %d conversations for one customer, want 1", len(all))
	}
}

// The claim is the lock. Two runs reaching one appointment in the same minute
// both try to write the row, and the database refuses the second — which is
// the whole of the double-send protection.
func TestASecondClaimForTheSameAppointmentIsRefused(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	accountID := connectNumber(t, app, alice)
	store := reminderjob.NewStore(app)
	at := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)

	if _, err := store.Claim(context.Background(), claimFor("evt1", alice, accountID, at)); err != nil {
		t.Fatalf("the first claim: %v", err)
	}

	_, err := store.Claim(context.Background(), claimFor("evt1", alice, accountID, at.Add(time.Minute)))
	if !errors.Is(err, reminders.ErrAlreadyClaimed) {
		t.Fatalf("the second claim returned %v, want reminders.ErrAlreadyClaimed", err)
	}

	if rows := reminderRows(t, app, "evt1"); len(rows) != 1 {
		t.Errorf("there are %d reminder rows for one appointment, want 1", len(rows))
	}
}

// The predicate on the index is what keeps it off everything else: two
// ordinary messages about one appointment, and a failed reminder beside a
// sent one, all coexist.
func TestTheClaimIndexOnlyBindsSentReminders(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")

	other := func(messageType, status string) error {
		model, err := app.FindCollectionByNameOrId("whatsapp_messages")
		if err != nil {
			return err
		}
		record := core.NewRecord(model)
		record.Set("user_id", alice)
		record.Set("to_number", "5216641234567")
		record.Set("event_id", "evt1")
		record.Set("message_type", messageType)
		record.Set("status", status)

		return app.Save(record)
	}

	for _, pair := range [][2]string{
		{"text", ""},
		{"text", ""},
		{"reminder", "failed"},
		{"reminder", "failed"},
		{"reminder", "sent"},
	} {
		if err := other(pair[0], pair[1]); err != nil {
			t.Fatalf("saving a %s/%s row for the same appointment was refused: %v", pair[0], pair[1], err)
		}
	}

	if err := other("reminder", "sent"); err == nil {
		t.Fatal("a second sent reminder for one appointment was accepted")
	}
}

// The predicate this whole design rests on is a unique-violation check, and
// app.Save does not return the error you would expect: PocketBase replaces
// the driver error with an ozzo validation error, so an errors.As against the
// SQLite type is false. A detector written that way would never fire, and a
// reminder another run already owned would surface as a hard failure instead
// of a silent skip — the protection inverted.
//
// This is a genuine double insert, not a mocked error.
func TestIsUniqueViolationRecognisesWhatAppSaveActuallyReturns(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")

	model, err := app.FindCollectionByNameOrId("whatsapp_messages")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	write := func() error {
		record := core.NewRecord(model)
		record.Set("user_id", alice)
		record.Set("to_number", "5216641234567")
		record.Set("event_id", "evt1")
		record.Set("message_type", "reminder")
		record.Set("status", "sent")

		return app.Save(record)
	}

	if err := write(); err != nil {
		t.Fatalf("the first insert: %v", err)
	}

	err = write()
	if err == nil {
		t.Fatal("the second insert succeeded; the partial unique index is not in force")
	}

	if !pbstore.IsUniqueViolation(err) {
		t.Errorf("IsUniqueViolation(%#v) = false, want true", err)
	}

	// The two shapes that make a naive detector wrong, asserted so that a
	// future rewrite of IsUniqueViolation cannot quietly drop the one the
	// production path actually produces.
	var driverError *sqlite.Error
	if errors.As(err, &driverError) {
		t.Error("app.Save returned a driver error; the comment explaining why this detector is not errors.As is now wrong")
	}
	var validationErrors validation.Errors
	if !errors.As(err, &validationErrors) {
		t.Errorf("app.Save returned %T, want an ozzo validation.Errors", err)
	}

	for _, notAViolation := range []error{nil, errors.New("the database went away"), context.Canceled} {
		if pbstore.IsUniqueViolation(notAViolation) {
			t.Errorf("IsUniqueViolation(%v) = true, want false", notAViolation)
		}
	}
}

// A failed send has to release its claim, or one transient Graph failure
// would suppress that reminder forever — which is today's behaviour.
func TestMarkFailedReleasesTheClaimSoTheNextRunCanTryAgain(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	accountID := connectNumber(t, app, alice)
	store := reminderjob.NewStore(app)
	at := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)

	claimID, err := store.Claim(context.Background(), claimFor("evt1", alice, accountID, at))
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := store.MarkFailed(context.Background(), claimID); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	if _, err := store.Claim(context.Background(), claimFor("evt1", alice, accountID, at.Add(5*time.Minute))); err != nil {
		t.Fatalf("the retry claim was refused: %v, want the failed attempt to have released it", err)
	}

	rows := reminderRows(t, app, "evt1")
	if len(rows) != 2 {
		t.Fatalf("there are %d reminder rows, want the failed attempt and the retry", len(rows))
	}
	if rows[0].GetString("status") != "failed" || rows[1].GetString("status") != "sent" {
		t.Errorf("statuses are %q and %q, want failed then sent",
			rows[0].GetString("status"), rows[1].GetString("status"))
	}

	// And the attempt is now countable, which is what bounds the retry.
	attempts, err := store.ReminderAttempts(context.Background(), []string{"evt1"})
	if err != nil {
		t.Fatalf("ReminderAttempts: %v", err)
	}
	if attempts["evt1"].Failures != 1 {
		t.Errorf("Failures = %d, want 1", attempts["evt1"].Failures)
	}
	if !attempts["evt1"].LastFailureAt.Equal(at) {
		t.Errorf("LastFailureAt = %s, want %s — the time the attempt was made", attempts["evt1"].LastFailureAt, at)
	}
}

func TestMarkSentRecordsTheWhatsAppMessageId(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	accountID := connectNumber(t, app, alice)
	store := reminderjob.NewStore(app)
	at := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)

	claimID, err := store.Claim(context.Background(), claimFor("evt1", alice, accountID, at))
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := store.MarkSent(context.Background(), claimID, "wamid.HBgLNTIx"); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	row := reminderRows(t, app, "evt1")[0]
	if row.GetString("message_id") != "wamid.HBgLNTIx" {
		t.Errorf("message_id = %q, want the id Meta returned", row.GetString("message_id"))
	}
	if row.GetString("status") != "sent" {
		t.Errorf("status = %q, want sent", row.GetString("status"))
	}
}

// The job has to actually be on the cron, under an id of this app's own: a
// typo in the expression would leave reminders silently never running, and
// PocketBase owns the ids wrapped in double underscores.
func TestTheRunIsScheduledOnceAMinute(t *testing.T) {
	app := newApp(t)
	reminderjob.Register(app, meta.New("", ""))

	var found bool
	for _, job := range app.Cron().Jobs() {
		if job.Id() != reminderjob.JobID {
			continue
		}
		found = true
		if job.Expression() != reminderjob.Schedule {
			t.Errorf("the reminder job runs on %q, want %q", job.Expression(), reminderjob.Schedule)
		}
	}

	if !found {
		t.Errorf("no cron job is registered as %q", reminderjob.JobID)
	}

	// PocketBase starts its cron from an OnServe hook, so a bootstrapped app
	// that is not serving has the job registered and not running — which is
	// what keeps `migrate` and `seed` from sending anything.
	if app.Cron().HasStarted() {
		t.Error("the cron is running outside serve")
	}
}
