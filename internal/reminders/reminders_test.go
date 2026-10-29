package reminders_test

import (
	"context"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// tijuana is the default reminder zone, and the one every case here uses:
// UTC-7 in summer, UTC-8 in winter, so a reminder formatted in it is visibly
// not the stored instant.
const tijuana = "America/Tijuana"

func settingsFor(user string, lead time.Duration) reminders.Settings {
	return reminders.Settings{
		UserID:           user,
		LeadTime:         lead,
		TemplateName:     "appointment_reminder",
		TemplateLanguage: "es",
		Timezone:         tijuana,
	}
}

func appointment(id, user, title, phone string, start time.Time) reminders.Appointment {
	return reminders.Appointment{
		ID:          id,
		UserID:      user,
		Title:       title,
		ClientPhone: phone,
		StartTime:   start,
	}
}

func connected(user string) *reminders.Account {
	return &reminders.Account{
		ID:            "acct-" + user,
		PhoneNumberID: "200000000000002",
		AccessToken:   "FAKE-TOKEN",
	}
}

// A reminder that has fallen due is claimed first and only then sent, with the
// client's name, the date and the time as the three body parameters — spelled
// in the owner's zone, not in UTC.
func TestADueReminderIsClaimedThenSentWithNameDateAndTime(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
	// 16:30 UTC is 08:30 in Tijuana on this date (PST, UTC-8).
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:30:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}

	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 1 {
		t.Fatalf("claims = %d, want 1 (%+v)", len(store.claims), store.claims)
	}
	if got := store.claims[0].AppointmentID; got != "evt1" {
		t.Errorf("claimed appointment = %q, want evt1", got)
	}

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1", len(sender.sends))
	}
	sent := sender.sends[0]
	if sent.to != "5216641234567" {
		t.Errorf("to = %q, want the client's phone", sent.to)
	}
	if sent.template != "appointment_reminder" || sent.language != "es" {
		t.Errorf("template = %q/%q, want appointment_reminder/es", sent.template, sent.language)
	}
	want := []string{"Ana Lopez", "03/11/2026", "08:30"}
	if len(sent.parameters) != 3 {
		t.Fatalf("parameters = %v, want 3 of them", sent.parameters)
	}
	for i := range want {
		if sent.parameters[i] != want[i] {
			t.Errorf("parameter {{%d}} = %q, want %q", i+1, sent.parameters[i], want[i])
		}
	}

	if got := store.sent["claim-evt1"]; got != "wamid.5216641234567" {
		t.Errorf("the claim was not completed with the message id: %q", got)
	}
	if summary.Sent != 1 {
		t.Errorf("summary.Sent = %d, want 1", summary.Sent)
	}
}
