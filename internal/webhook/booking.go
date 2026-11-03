package webhook

import (
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/isotime"
)

// AppointmentLength is how long an appointment the assistant writes is
// assumed to run for.
//
// A conversation says when it starts and practically never says when it
// ends, so something has to be assumed, and the end is a required column.
// An hour is what the dialog already offers a member of staff by default, so
// an appointment written from a conversation looks like one booked at the
// desk rather than announcing itself by being a different length.
const AppointmentLength = time.Hour

// ExtractedAppointment is an appointment a conversation agreed on, as the
// model read it. It is what Deps.ExtractAppointment returns, and it is not
// yet an appointment: nothing here has decided whether it is certain enough
// to be one.
type ExtractedAppointment struct {
	// ClientName is the customer's name as they gave it, which may be empty.
	ClientName string

	// StartsAt is the moment agreed.
	StartsAt time.Time

	// Certain is whether the conversation named a specific date, a specific
	// time, and assent from both sides.
	Certain bool

	// Summary is one line saying what was agreed or asked for.
	Summary string
}

// Appointment is an appointment to write.
//
// Everything on it is decided here rather than by whatever stores it: the
// decision about what an agreement in a WhatsApp thread means is this
// package's, and it is answerable with no database precisely because this
// type is the whole of its output.
type Appointment struct {
	ClientName  string
	ClientPhone string

	// StartsAt and EndsAt are in the format internal/isotime defines, which
	// is what the rest of the app sorts and range-filters these as.
	StartsAt string
	EndsAt   string

	Status      appointments.Status
	CreatedBy   appointments.Actor
	StatusSetBy appointments.Actor
	StatusSetAt string

	// ConversationID is the thread this was read out of. It is what answers
	// "why does this exist?", and it is also the key the duplicate rule is
	// enforced on.
	ConversationID string

	// Notes is the model's one-line account of what was agreed, for the
	// person who later has to decide whether to keep this.
	Notes string
}

// Appointment turns what the model read into the appointment to write.
//
// The split on Certain is the whole design of this feature. A conversation
// that named a day, a time and assent from both sides becomes a pending
// appointment, silently, which is the point of it. Anything partial or
// hedged becomes a proposal instead: murky cases are exactly where phantom
// appointments come from, so they land somewhere a person looks rather than
// in the calendar as a booking the clinic will plan its day around.
func (e ExtractedAppointment) Appointment(customerPhone, conversationID string, at time.Time) Appointment {
	status := appointments.Proposed
	if e.Certain {
		status = appointments.Pending
	}

	name := e.ClientName
	if name == "" {
		// client_name is a required column, so an agreement reached before
		// anybody gave a name would be rejected outright and the
		// appointment lost. The number the clinic can call is a worse name
		// than a name and a far better one than nothing.
		name = customerPhone
	}

	return Appointment{
		ClientName:  name,
		ClientPhone: customerPhone,
		StartsAt:    isotime.Format(e.StartsAt),
		EndsAt:      isotime.Format(e.StartsAt.Add(AppointmentLength)),
		Status:      status,

		// Both, and not only the status: an appointment that records who
		// put it in its current state but not who created it cannot be told
		// apart from one a member of staff booked and the assistant later
		// touched.
		CreatedBy:   appointments.Assistant,
		StatusSetBy: appointments.Assistant,
		StatusSetAt: isotime.Format(at),

		ConversationID: conversationID,
		Notes:          e.Summary,
	}
}
