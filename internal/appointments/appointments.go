// Package appointments holds the words an appointment's lifecycle is written
// in: what state it is in, and who put it there.
//
// They are a package of their own because three unrelated layers have to
// agree on the exact strings — the migration that defines the column and
// backfills it, the scheduler that refuses to remind about half of them, and
// the webhook that will write them when a patient taps a button. The
// scheduler is one of the packages internal/arch keeps free of PocketBase, so
// it cannot reach into migrations for them, and the alternative to a shared
// package is the same four literals spelled out in three places. A
// disagreement between those spellings does not fail anything: it silently
// stops a reminder, or sends one for an appointment that is not happening.
//
// Nothing here imports anything. That is deliberate — see internal/arch.
package appointments

// Status is where an appointment stands.
//
// The four are a closed set, and the order below is the life of a booking:
// an assistant may propose one, staff book one, the patient confirms, and
// either side can cancel.
type Status string

const (
	// Proposed is the assistant's reading of a conversation: it believes an
	// appointment was agreed but is not certain. It is not a commitment, and
	// nothing may act on it as though it were one.
	Proposed Status = "proposed"

	// Pending is a real appointment the patient has not confirmed. Every
	// appointment staff book starts here, and so did every appointment
	// booked before this column existed.
	Pending Status = "pending"

	// Confirmed is a real appointment the patient has said yes to.
	Confirmed Status = "confirmed"

	// Cancelled is not happening.
	Cancelled Status = "cancelled"
)

// AllowsReminders reports whether a reminder may go out for an appointment in
// this state.
//
// It is an allow-list rather than a pair of exclusions on purpose. The states
// that may be reminded about are the two that describe a booking somebody is
// expected at; anything else — a proposal nobody agreed to, a cancellation, a
// value some future migration adds and forgets to think about here — stays
// silent until it is deliberately let through. Telling a patient to come to
// an appointment that is not happening is the one failure here nobody can
// take back.
func (s Status) AllowsReminders() bool {
	return s == Pending || s == Confirmed
}

// Actor is who did something to an appointment.
//
// "Who said so" is the first question asked of a screen that says cancelled,
// and the three answers are not interchangeable: staff cancelling is a
// decision, the patient cancelling is news, and the assistant doing it is
// something a human may want to look at.
type Actor string

const (
	// Staff is somebody at the business, working in this app.
	Staff Actor = "staff"

	// Patient is the person the appointment is with, acting over WhatsApp.
	Patient Actor = "patient"

	// Assistant is the AI, acting on a conversation it read.
	Assistant Actor = "assistant"
)
