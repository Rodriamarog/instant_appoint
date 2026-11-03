package webhook_test

import (
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// agreed is a clear agreement: a date, a time, and the model saying both
// sides settled on it.
var agreed = webhook.ExtractedAppointment{
	ClientName: "Ana Ruiz",
	StartsAt:   time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC),
	Certain:    true,
	Summary:    "Limpieza dental",
}

const (
	customerPhone  = "5216641234567"
	conversationID = "conv-1"
)

var wroteAt = time.Date(2026, 10, 28, 19, 36, 0, 0, time.UTC)

// The whole point of the split: a conversation that named a day, named a
// time and got assent from both sides produces a real appointment, silently,
// which is the feature.
func TestAClearAgreementBecomesAPendingAppointment(t *testing.T) {
	written := agreed.Appointment(customerPhone, conversationID, wroteAt)

	if written.Status != appointments.Pending {
		t.Errorf("Status = %q, want %q", written.Status, appointments.Pending)
	}
	if written.ClientName != "Ana Ruiz" {
		t.Errorf("ClientName = %q, want Ana Ruiz", written.ClientName)
	}
	if written.ClientPhone != customerPhone {
		t.Errorf("ClientPhone = %q, want %q", written.ClientPhone, customerPhone)
	}
	if want := "2026-10-30T22:00:00.000Z"; written.StartsAt != want {
		t.Errorf("StartsAt = %q, want %q", written.StartsAt, want)
	}
	// An hour, the same length the dialog offers a member of staff by
	// default. The conversation says when it starts and never says when it
	// ends, so a guess is unavoidable; this is the one already on the
	// screen everywhere else.
	if want := "2026-10-30T23:00:00.000Z"; written.EndsAt != want {
		t.Errorf("EndsAt = %q, want %q", written.EndsAt, want)
	}
}

// The other half. "Sometime Tuesday" is exactly where phantom appointments
// come from, so it lands somewhere a person looks rather than in the
// calendar as a booking.
func TestAHedgedAgreementBecomesAProposal(t *testing.T) {
	hedged := agreed
	hedged.Certain = false

	if status := hedged.Appointment(customerPhone, conversationID, wroteAt).Status; status != appointments.Proposed {
		t.Errorf("Status = %q, want %q", status, appointments.Proposed)
	}
}

// Every appointment this writes has to answer "why does this exist?", and
// both halves of the answer are recorded here: that the assistant wrote it,
// and which conversation it read.
func TestEveryAppointmentItWritesRecordsTheAssistantAndTheConversation(t *testing.T) {
	written := agreed.Appointment(customerPhone, conversationID, wroteAt)

	if written.CreatedBy != appointments.Assistant {
		t.Errorf("CreatedBy = %q, want %q", written.CreatedBy, appointments.Assistant)
	}
	if written.StatusSetBy != appointments.Assistant {
		t.Errorf("StatusSetBy = %q, want %q", written.StatusSetBy, appointments.Assistant)
	}
	if want := "2026-10-28T19:36:00.000Z"; written.StatusSetAt != want {
		t.Errorf("StatusSetAt = %q, want %q", written.StatusSetAt, want)
	}
	if written.ConversationID != conversationID {
		t.Errorf("ConversationID = %q, want %q", written.ConversationID, conversationID)
	}
	if written.Notes != "Limpieza dental" {
		t.Errorf("Notes = %q, want the model's summary", written.Notes)
	}
}

// client_name is a required column. A thread that settles on a time before
// anybody says who it is for would therefore be rejected outright and the
// appointment lost — so the number the clinic can call stands in for the
// name it does not have.
func TestAnAppointmentWithNoNameIsFiledUnderThePhoneNumber(t *testing.T) {
	anonymous := agreed
	anonymous.ClientName = ""

	if name := anonymous.Appointment(customerPhone, conversationID, wroteAt).ClientName; name != customerPhone {
		t.Errorf("ClientName = %q, want the customer's number %q", name, customerPhone)
	}
}
