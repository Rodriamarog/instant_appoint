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

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
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

	// DefaultLanguage is the template variant the business configured, as
	// Meta spells it: "es", "en_US".
	//
	// It used to be the language every one of this business's reminders went
	// out in. It is now only a default: the language belongs to the
	// appointment, and this is what an appointment that carries none of its
	// own falls back to — which is what every row written before the column
	// existed looked like until the migration copied this value onto it.
	// Keeping the fallback means an older client that writes an appointment
	// without a language still gets the behaviour the clinic had, rather
	// than silence.
	DefaultLanguage string

	// Timezone is an IANA zone name. The date and the time in the reminder
	// are spelled in it; the appointment itself is a UTC instant.
	Timezone string
}

// Appointment is a booking a reminder may be due for.
type Appointment struct {
	ID     string
	UserID string

	// ClientName is who the booking is with. It goes out as the template's
	// {{1}}, which the customer reads as their own name.
	ClientName string

	// ClientPhone is where the reminder goes, in Meta's digits-only form.
	ClientPhone string

	StartTime time.Time

	// Language is the template variant this patient is written to in, as
	// Meta spells it: "es", "en_US". Empty falls back to the business's
	// Settings.DefaultLanguage.
	//
	// It is the patient's language, not the clinic's. A Tijuana clinic
	// treats people from both sides of the border, and a Mexican patient who
	// prefers English is unremarkable here — which is why staff can set it
	// rather than it being inferred from the phone number and locked.
	Language string

	// Status decides whether a reminder is owed at all — see
	// appointments.Status.AllowsReminders. A Store that leaves it blank gets
	// nothing sent for that appointment: the column it comes from has a
	// create-time default, so a blank here means something wrote a state
	// this package has never been told how to read, and guessing at it is
	// how a patient is told to come to an appointment that is not happening.
	Status appointments.Status
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

	// NotRemindable counts the appointments whose status rules a reminder
	// out: a proposal nobody has agreed to, and a booking that has been
	// cancelled. It is its own counter rather than part of Skipped because
	// it is the one reason a clinic may want to go and look at.
	NotRemindable int

	// NoLanguage counts the reminders held back because the language the
	// appointment asks for has no row in the spelling table — pt_BR, which
	// the template form offers and this app has no month names for.
	//
	// Its own counter rather than part of Skipped because it is a clinic's
	// configuration asking for something this build cannot write, and the
	// only way to fix it is for somebody to go and look.
	NoLanguage int

	// GaveUp counts reminders past MaxAttempts.
	GaveUp int

	// Skipped counts the ordinary reasons: not due yet, already sent, no
	// phone number to send to, waiting out a backoff.
	Skipped int
}

// The time as a reminder spells it.
//
// 24-hour, which is what lets one rendering serve every reader: 15:30 has no
// second meaning, where "3:30" is twelve hours wide. Digits alone could not do
// the same for the date — see spellings.
const timeLayout = "15:04"

// language is which of the two languages a reminder's date is spelled in.
//
// Spanish is the zero value because it is what every reminder this app sent
// before an appointment could carry a language of its own.
type language int

const (
	spanish language = iota
	english
)

// spelling is one language's worth of calendar vocabulary, and the name Meta
// knows that language by.
type spelling struct {
	// variant is the template's language code, which a send names and Meta
	// picks the approved wording by. It sits in the same row as the month
	// names deliberately: the wording Meta assembles and the date this app
	// hands it have to be the same language, and keeping the two in separate
	// tables is how they come apart.
	variant string

	months [12]string

	// date is a layout for fmt.Sprintf taking the day, the month's name and
	// the year — in that order as arguments, in any order in the output,
	// which is the point of the indexes.
	date string
}

// How a date is written, per language.
//
// The reminder used to interpolate "02/01/2006". A patient in Tijuana reads
// "03/11/2026" as 3 November and one from San Diego reads it as 11 March, and
// a border clinic has both on one list — same digits, four months apart, in
// the one message whose entire job is to put somebody in a chair on the right
// day. Naming the month is the only form of the date that cannot be read two
// ways, so the month has to be a word, and a word is in some language.
//
// Twenty-four of them, though. That is a table, not a reason to carry
// golang.org/x/text and a locale setting nobody has asked for — the constraint
// that chose the digits in the first place still holds, and this satisfies it.
var spellings = [...]spelling{
	spanish: {
		variant: "es",
		months: [12]string{
			"enero", "febrero", "marzo", "abril", "mayo", "junio",
			"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
		},
		date: "%[1]d de %[2]s de %[3]d",
	},
	english: {
		// en_US and not en: it is what the template form offers, so it is
		// what a business's approved variant is actually called, and Meta
		// matches the code it is given rather than the language in it.
		variant: "en_US",
		months: [12]string{
			"January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December",
		},
		date: "%[2]s %[1]d, %[3]d",
	},
}

// languageFor is the variant code a reminder was asked for, as a language
// this app can write a date in.
//
// Not found is the whole point of the second return. The template form offers
// pt_BR and there is no Portuguese row here, so a clinic can configure a
// variant whose months this build does not know — and the wrong answer is to
// fall back to Spanish, which would put a Spanish month inside Portuguese
// wording and look entirely ordinary from the outside. The caller holds the
// reminder back instead.
func languageFor(variant string) (language, bool) {
	for index, spelling := range spellings {
		if spelling.variant == variant {
			return language(index), true
		}
	}

	return 0, false
}

// spellDate writes out the date a reminder names, with the month as a word.
func spellDate(local time.Time, lang language) string {
	chosen := spellings[lang]

	return fmt.Sprintf(chosen.date, local.Day(), chosen.months[local.Month()-1], local.Year())
}

// SpellMoment writes an instant the way the patient it is about reads it:
// the date with its month as a word, and the time on a 24-hour clock, both
// in the clinic's zone and in the variant asked for. ok is false for a
// variant this build has no calendar vocabulary for.
//
// Exported because the creation receipt names the same appointment this
// package will later remind about, and the two have to agree about what day
// it is. A second month table would be a second answer to "what is 30/10?"
// — which is the exact question the word-month spelling exists to settle —
// and it would drift the first time one of them gained a language.
func SpellMoment(at time.Time, location *time.Location, variant string) (date, clock string, ok bool) {
	lang, ok := languageFor(variant)
	if !ok {
		return "", "", false
	}

	local := at.In(location)

	return spellDate(local, lang), local.Format(timeLayout), true
}

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

		// Asked before anything else about this appointment, because it is
		// the only question whose answer is "this is not a reminder that
		// should ever exist". Everything below is about a reminder that is
		// owed and may not be due, already gone, or undeliverable.
		if !appointment.Status.AllowsReminders() {
			summary.NotRemindable++
			continue
		}

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

		// Resolved before anything is claimed or sent, because a reminder
		// whose date cannot be written is one that must not go out at all —
		// see Summary.NoLanguage. Whether Meta has actually approved this
		// template in this variant is a question only Meta can answer: an
		// unapproved one comes back refused and is recorded as a failed
		// attempt, which is the second half of the same guarantee. Between
		// them, no patient is sent a message in a language they did not ask
		// for.
		variant := appointment.Language
		if variant == "" {
			variant = settings.DefaultLanguage
		}
		lang, ok := languageFor(variant)
		if !ok {
			summary.NoLanguage++
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

		parameters := bodyParameters(appointment, location, lang)

		claimID, err := store.Claim(ctx, Claim{
			AppointmentID: appointment.ID,
			UserID:        appointment.UserID,
			AccountID:     account.ID,
			PhoneNumberID: account.PhoneNumberID,
			ToNumber:      appointment.ClientPhone,
			Content:       content(settings.TemplateName, variant, parameters),
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
			settings.TemplateName, variant, parameters)
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
func bodyParameters(appointment Appointment, location *time.Location, lang language) []string {
	local := appointment.StartTime.In(location)

	return []string{appointment.ClientName, spellDate(local, lang), local.Format(timeLayout)}
}

// content renders what was sent, for the stored row and the reminders page.
//
// Meta assembles the real text from the approved template, which this app
// never holds, so this records the template and its parameters rather than
// inventing a message that was never delivered.
//
// variant is the one the send asked for and not the business's setting: two
// patients of one clinic now receive different wording from the same
// template, and a row that named the setting would tell the clinic the wrong
// one.
func content(templateName, variant string, parameters []string) string {
	return fmt.Sprintf("%s (%s): %s", templateName, variant, strings.Join(parameters, " | "))
}
