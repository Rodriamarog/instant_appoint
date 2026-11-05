package webhook

import (
	"strings"
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

	// ClientPhone is a phone number the customer wrote in the conversation,
	// as the model read it, or empty. Only read on Messenger and Instagram,
	// where it is the one way the appointment can get a WhatsApp reminder;
	// on WhatsApp the conversation's own number is the fact.
	ClientPhone string
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

	// Language is the Meta template variant this patient is written to in
	// — "es", "en_US". It is the appointment's own and not the clinic's: a
	// Tijuana clinic treats people from both sides of the border, and the
	// reminder that goes out a day later reads this column.
	//
	// Guessed from the dialling prefix, which is right most of the time and
	// wrong often enough that staff can change it afterwards. Writing
	// nothing would be worse than guessing: the column's create-time
	// default is Spanish, so a US patient's whole thread would be in
	// English and both messages this app sends them would be in Spanish.
	Language string

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
		Language:    languageFor(customerPhone),
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

// The language codes an appointment can be born with, as Meta spells a
// template variant.
const (
	spanish = "es"
	english = "en_US"
)

// languageFor guesses the language a patient reads from their number's
// dialling prefix.
//
// It mirrors languageForCountryCode in web/src/lib/appointments.ts, which is
// what the dialog applies when a member of staff types a number, so an
// appointment the assistant writes and one typed at the desk for the same
// patient come out the same. Both are a guess, and both are a stored column
// staff can correct.
//
// Anything that is not a +1 number falls back to Spanish rather than being
// read as English: the clinic is in Tijuana, Spanish is the one variant its
// reminder template is certain to be approved in, and a wrong guess the
// other way costs a reminder that Meta refuses to deliver at all.
func languageFor(customerPhone string) string {
	if strings.HasPrefix(customerPhone, "1") {
		return english
	}

	return spanish
}

// digitsOnly reduces a phone number as somebody typed it to the digits Meta
// sends to, or "" when what is left is too short to include a country code.
// A ten-digit local number is not guessed at: prefixing the wrong country
// sends somebody else's reminder, and an empty phone just means staff add it.
func digitsOnly(phone string) string {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}

	if digits.Len() < 11 || digits.Len() > 15 {
		return ""
	}

	return digits.String()
}
