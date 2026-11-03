package reminders_test

import (
	"context"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// What an appointment's status is allowed to cost is a reminder, and the two
// directions of that are not symmetrical. Sending one too few is a patient
// who forgets; sending one too many is a clinic telling somebody to come to
// an appointment that was cancelled, or confirming one nobody agreed to. Each
// of the four states therefore gets its own case here rather than being
// covered in passing by a test about something else.

// run is the whole of a case: one person with reminders on, one appointment
// of the given status falling due now, one connected number.
func run(t *testing.T, status appointments.Status) (*fakeSender, reminders.Summary) {
	t.Helper()

	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}

	due := appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:30:00Z"))
	due.Status = status
	store.appointments = []reminders.Appointment{due}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}

	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	return sender, summary
}

func TestAProposedAppointmentIsNeverRemindedAbout(t *testing.T) {
	// A proposal is the assistant's reading of a conversation, not something
	// the clinic has agreed to. Reminding about it would tell the patient
	// they have an appointment on the strength of a guess.
	sender, summary := run(t, appointments.Proposed)

	if len(sender.sends) != 0 {
		t.Errorf("a proposed appointment was reminded about: %+v", sender.sends)
	}
	if summary.NotRemindable != 1 {
		t.Errorf("not_remindable = %d, want 1 — the run has to say why it sent nothing", summary.NotRemindable)
	}
}

func TestACancelledAppointmentIsNeverRemindedAbout(t *testing.T) {
	// The appointment is not happening. A reminder for it is the single most
	// visible way this app can be wrong at a patient.
	sender, summary := run(t, appointments.Cancelled)

	if len(sender.sends) != 0 {
		t.Errorf("a cancelled appointment was reminded about: %+v", sender.sends)
	}
	if summary.NotRemindable != 1 {
		t.Errorf("not_remindable = %d, want 1", summary.NotRemindable)
	}
}

func TestAPendingAppointmentIsRemindedAbout(t *testing.T) {
	// The ordinary case, and the one every appointment booked before this
	// column existed was migrated into: a real booking nobody has confirmed
	// yet, which is exactly what a reminder is for.
	sender, summary := run(t, appointments.Pending)

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1 — a pending appointment still needs its reminder", len(sender.sends))
	}
	if summary.NotRemindable != 0 {
		t.Errorf("not_remindable = %d, want 0", summary.NotRemindable)
	}
}

func TestAConfirmedAppointmentIsRemindedAbout(t *testing.T) {
	// Confirmed is not "done with": the patient said yes days ago and the
	// reminder is what gets them out of the house.
	sender, _ := run(t, appointments.Confirmed)

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1 — confirming an appointment must not switch its reminder off", len(sender.sends))
	}
}

func TestACancelledAppointmentDoesNotStopTheOnesAfterIt(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}

	cancelled := appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:20:00Z"))
	cancelled.Status = appointments.Cancelled
	store.appointments = []reminders.Appointment{
		cancelled,
		appointment("evt2", "alice", "Beto Ruiz", "5216647654321", at(t, "2026-11-03T16:30:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}

	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1", len(sender.sends))
	}
	if got := sender.sends[0].to; got != "5216647654321" {
		t.Errorf("sent to %q, want the appointment after the cancelled one", got)
	}
}
