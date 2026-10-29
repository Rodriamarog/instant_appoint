// Package reminders decides which appointment reminders are due and sends
// them, exactly once each.
//
// Nothing here knows about PocketBase, HTTP or Meta: a Store supplies the
// rows and a Sender delivers the template, both as interfaces. That is not
// decoration. The behaviour this package owns is a long list of cases — a
// lead time that reaches further than the window, an owner who disconnected
// their number, a send that fails three times, two runs racing the same
// minute — and the only way that list stays honest is if adding a case to it
// costs one Go test and no database.
//
// Reminders cannot be free text. Outside the 24-hour window that a customer's
// own message opens, WhatsApp delivers nothing but a template Meta has
// approved, so a reminder is a template name plus three positional
// parameters: the client's name, the date and the time.
package reminders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrAlreadyClaimed is what a Store returns when the reminder row it was
// asked to write is already there.
//
// It is the ordinary outcome of two runs reaching one appointment in the same
// minute, not a failure: the other run owns the send, and this one steps away
// without touching Meta.
var ErrAlreadyClaimed = errors.New("reminders: another run has already claimed this reminder")

// Settings is one person's reminder configuration.
type Settings struct {
	UserID string

	// LeadTime is how far before the appointment the reminder goes out.
	LeadTime time.Duration

	// TemplateName is the approved Meta template. Empty means nobody has
	// chosen one yet, which is not something to guess at.
	TemplateName string

	// TemplateLanguage is the template's language variant, e.g. "es".
	TemplateLanguage string

	// Timezone is an IANA zone name. The date and the time in the reminder
	// are spelled in it; the appointment itself is a UTC instant.
	Timezone string
}

// Appointment is a calendar event a reminder may be due for.
type Appointment struct {
	ID     string
	UserID string

	// Title is the client's name. The event dialog puts it there.
	Title string

	// ClientPhone is where the reminder goes, in Meta's digits-only form.
	ClientPhone string

	StartTime time.Time
}

// Account is the connected WhatsApp number a reminder is sent from.
type Account struct {
	ID string

	// PhoneNumberID is the sending number's Meta id — not the WABA id.
	PhoneNumberID string

	AccessToken string
}

// Attempt is what has already happened to one appointment's reminder.
//
// There is no attempt counter in the schema: every attempt leaves a row
// behind, so counting the failed ones is counting the attempts, and it
// survives a restart for free.
type Attempt struct {
	// Sent is true once a reminder for this appointment has gone out.
	Sent bool

	// Incomplete says that sent row carries no WhatsApp message id, so the
	// claim was written but never finished. See Abandoned.
	Incomplete bool

	// ClaimedAt is when that unfinished claim was written.
	ClaimedAt time.Time

	// Failures is how many sends have been tried and failed.
	Failures int

	// LastFailureAt is when the most recent of them was tried.
	LastFailureAt time.Time
}

// ReclaimAfter is how long an unfinished claim may sit before another run is
// allowed to take it over.
//
// A claim is written with status='sent' *before* Meta is called, which is what
// makes the unique index a reservation. If the process dies in between, the
// row is left saying "sent" with no message id and no send having happened —
// and because the index refuses any second status='sent' row, nothing could
// ever claim that appointment again. The reminder would be lost silently and
// permanently, with no audit trail.
//
// The window has to be far longer than a healthy send can take. The Graph
// client's own timeout is 10s and a whole run is abandoned after 50s, so
// fifteen minutes is some twenty times the longest a live claim can
// legitimately stay open: nothing merely slow is ever mistaken for something
// dead.
const ReclaimAfter = 15 * time.Minute

// Abandoned reports a claim that was written and never finished, long enough
// ago that whatever was holding it cannot still be alive.
//
// The tradeoff here is deliberate and unavoidable: claim-then-send is
// at-most-once, send-then-claim is at-least-once, and no ordering is both. A
// send that did reach Meta but died before its message id was recorded will go
// out a second time. For an appointment reminder a rare duplicate after a
// crash is plainly better than silence.
func (a Attempt) Abandoned(now time.Time) bool {
	return a.Sent && a.Incomplete && !now.Before(a.ClaimedAt.Add(ReclaimAfter))
}

// Claim is the row written before Meta is called.
//
// Writing it is what reserves the send: the database's unique index refuses a
// second one, so the row is the lock rather than a flag checked before taking
// it.
type Claim struct {
	AppointmentID string
	UserID        string

	// AccountID and PhoneNumberID record which connected number carried it.
	AccountID     string
	PhoneNumberID string

	// ToNumber is the customer, which is also what their conversation is
	// keyed by.
	ToNumber string

	// Content is a human-readable rendering of what was sent. The real text
	// is assembled by Meta from the approved template, which this app never
	// sees, so this records the template and the parameters instead of
	// pretending to be the message.
	Content string

	At time.Time
}

// Store is everything this package needs from the database.
type Store interface {
	// ActiveSettings returns the configuration of everybody who has
	// reminders switched on. It is read once per run.
	ActiveSettings(ctx context.Context) ([]Settings, error)

	// AppointmentsStartingBetween returns every appointment, for everybody,
	// starting in [from, to].
	AppointmentsStartingBetween(ctx context.Context, from, to time.Time) ([]Appointment, error)

	// AccountFor returns the person's connected Cloud API account, or nil
	// when they have none.
	AccountFor(ctx context.Context, userID string) (*Account, error)

	// ReminderAttempts reports what has already been tried, keyed by
	// appointment id. Appointments nothing has been tried for may be absent.
	ReminderAttempts(ctx context.Context, appointmentIDs []string) (map[string]Attempt, error)

	// Claim writes the sent row and returns its id, or ErrAlreadyClaimed if
	// another run got there first.
	Claim(ctx context.Context, claim Claim) (string, error)

	// MarkSent records the WhatsApp message id against a claim.
	MarkSent(ctx context.Context, claimID, messageID string) error

	// MarkFailed turns a claim into a failed attempt, which releases it for
	// another try.
	MarkFailed(ctx context.Context, claimID string) error
}

// Sender delivers one approved template.
type Sender interface {
	SendTemplate(ctx context.Context, account Account, to, templateName, languageCode string, bodyParameters []string) (messageID string, err error)
}

// Summary counts what one run did, for the log line it leaves behind.
type Summary struct {
	// Considered is how many appointments in the window belonged to somebody
	// with reminders switched on.
	Considered int

	Sent   int
	Failed int

	// Claimed counts reminders another run owned.
	Claimed int

	NoAccount   int
	NoTemplate  int
	BadTimezone int

	// GaveUp counts reminders past MaxAttempts.
	GaveUp int

	// Skipped counts the ordinary reasons: not due yet, already sent, no
	// phone number to send to, waiting out a backoff.
	Skipped int
}

// The date and the time as a reminder spells them.
//
// Day-first and 24-hour, which is unambiguous and matches the Spanish-speaking
// default this app ships with. Deliberately not locale-aware: month names in
// the recipient's language would mean carrying golang.org/x/text and a locale
// setting nobody has asked for, and the template's own words around these two
// values are already written by the business in its own language.
const (
	dateLayout = "02/01/2006"
	timeLayout = "15:04"
)

// How a failing send is bounded.
//
// The reminder row's unique index is predicated on status='sent', so a failed
// send releases its claim and the next run picks it up again. Without a limit
// a permanently failing send — a revoked token, a number Meta will not deliver
// to — would retry every sixty seconds until the appointment passed. Three
// attempts spread over a quarter of an hour is enough to ride out a transient
// Graph failure and short enough that nothing is still trying an hour later.
const (
	MaxAttempts  = 3
	RetryBackoff = 5 * time.Minute
)

// retryDelay is how long to wait after the nth failure before trying again.
func retryDelay(failures int) time.Duration {
	if failures < 1 {
		return 0
	}

	return RetryBackoff << (failures - 1)
}

// Run sends every reminder that has fallen due as of now.
//
// now is a parameter rather than read here so that the window arithmetic is
// testable at all: every case in this package's tests fixes it.
func Run(ctx context.Context, store Store, sender Sender, now time.Time) (Summary, error) {
	var summary Summary

	active, err := store.ActiveSettings(ctx)
	if err != nil {
		return summary, fmt.Errorf("load reminder settings: %w", err)
	}
	if len(active) == 0 {
		return summary, nil
	}

	// Settings are read once and indexed, not looked up per appointment. The
	// scheduler this replaces issued one query per event every minute.
	byUser := make(map[string]Settings, len(active))
	var furthest time.Duration
	for _, settings := range active {
		byUser[settings.UserID] = settings
		if settings.LeadTime > furthest {
			furthest = settings.LeadTime
		}
	}

	// The window reaches as far ahead as the longest lead time anybody has
	// configured, because that is the first moment one of their reminders can
	// fall due. The scheduler this replaces used a fixed 25 hours, which
	// silently capped every lead time at about 1500 minutes.
	//
	// The near edge is now: an appointment already under way is past being
	// reminded about. A reminder whose moment passed while the appointment is
	// still ahead is still sent, late, which is what the reminders page shows
	// as due.
	appointments, err := store.AppointmentsStartingBetween(ctx, now, now.Add(furthest))
	if err != nil {
		return summary, fmt.Errorf("load appointments: %w", err)
	}

	candidates := make([]Appointment, 0, len(appointments))
	ids := make([]string, 0, len(appointments))
	for _, appointment := range appointments {
		if _, ok := byUser[appointment.UserID]; !ok {
			continue
		}
		candidates = append(candidates, appointment)
		ids = append(ids, appointment.ID)
	}
	summary.Considered = len(candidates)
	if len(candidates) == 0 {
		return summary, nil
	}

	attempts, err := store.ReminderAttempts(ctx, ids)
	if err != nil {
		return summary, fmt.Errorf("load what has already been tried: %w", err)
	}

	// One account lookup per person per run, not per appointment.
	accounts := map[string]*Account{}

	for _, appointment := range candidates {
		if err := ctx.Err(); err != nil {
			return summary, err
		}

		settings := byUser[appointment.UserID]
		attempt := attempts[appointment.ID]

		// An abandoned claim reads as sent and is not: it is a crash caught
		// mid-send, and letting it through here is what stops one dead
		// process from suppressing a reminder forever.
		alreadySent := attempt.Sent && !attempt.Abandoned(now)

		if appointment.ClientPhone == "" || alreadySent || now.Before(appointment.StartTime.Add(-settings.LeadTime)) {
			summary.Skipped++
			continue
		}

		if settings.TemplateName == "" {
			summary.NoTemplate++
			continue
		}

		if attempt.Failures >= MaxAttempts {
			summary.GaveUp++
			continue
		}
		if attempt.Failures > 0 && now.Before(attempt.LastFailureAt.Add(retryDelay(attempt.Failures))) {
			summary.Skipped++
			continue
		}

		location, err := time.LoadLocation(settings.Timezone)
		if err != nil {
			summary.BadTimezone++
			continue
		}

		account, ok := accounts[appointment.UserID]
		if !ok {
			account, err = store.AccountFor(ctx, appointment.UserID)
			if err != nil {
				return summary, fmt.Errorf("look up the connected account for %s: %w", appointment.UserID, err)
			}
			accounts[appointment.UserID] = account
		}
		if account == nil {
			summary.NoAccount++
			continue
		}

		parameters := bodyParameters(appointment, location)

		claimID, err := store.Claim(ctx, Claim{
			AppointmentID: appointment.ID,
			UserID:        appointment.UserID,
			AccountID:     account.ID,
			PhoneNumberID: account.PhoneNumberID,
			ToNumber:      appointment.ClientPhone,
			Content:       content(settings, parameters),
			At:            now,
		})
		switch {
		case errors.Is(err, ErrAlreadyClaimed):
			summary.Claimed++
			continue

		case err != nil:
			return summary, fmt.Errorf("claim the reminder for %s: %w", appointment.ID, err)
		}

		messageID, err := sender.SendTemplate(ctx, *account, appointment.ClientPhone,
			settings.TemplateName, settings.TemplateLanguage, parameters)
		if err != nil {
			summary.Failed++
			if markErr := store.MarkFailed(ctx, claimID); markErr != nil {
				return summary, fmt.Errorf("record the failed reminder for %s: %w", appointment.ID, markErr)
			}
			continue
		}

		if err := store.MarkSent(ctx, claimID, messageID); err != nil {
			return summary, fmt.Errorf("record the sent reminder for %s: %w", appointment.ID, err)
		}
		summary.Sent++
	}

	return summary, nil
}

// bodyParameters are the template's {{1}}, {{2}} and {{3}}, in order.
func bodyParameters(appointment Appointment, location *time.Location) []string {
	local := appointment.StartTime.In(location)

	return []string{appointment.Title, local.Format(dateLayout), local.Format(timeLayout)}
}

// content renders what was sent, for the stored row and the reminders page.
//
// Meta assembles the real text from the approved template, which this app
// never holds, so this records the template and its parameters rather than
// inventing a message that was never delivered.
func content(settings Settings, parameters []string) string {
	return fmt.Sprintf("%s (%s): %s", settings.TemplateName, settings.TemplateLanguage, strings.Join(parameters, " | "))
}
