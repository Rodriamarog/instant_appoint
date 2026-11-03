package reminders_test

import (
	"context"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// A Tijuana clinic treats people from both sides of the border, so the
// language a reminder is written in belongs to the appointment and not to the
// business. Both halves have to move together: the variant Meta is asked for
// decides the wording it assembles, and the date parameter this app supplies
// decides the month name inside it. One without the other produces the exact
// message this ticket exists to stop — English wording with a Spanish month,
// or the reverse.
func TestEachReminderGoesOutInItsOwnAppointmentsLanguage(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	// 16:30 UTC is 08:30 in Tijuana on this date (PST, UTC-8).
	start := at(t, "2026-11-03T16:30:00Z")

	for _, test := range []struct {
		name     string
		language string
		wantDate string
	}{
		// Day first, lower-case month, the preposition twice: the only
		// spelling of 3 November a Spanish reader does not have to decode.
		{"a patient who reads Spanish", "es", "3 de noviembre de 2026"},

		// Month first and a comma, because the English readers here are the
		// ones from San Diego.
		{"a patient who reads English", "en_US", "November 3, 2026"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newStore()
			store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
			booking := appointment("evt1", "alice", "Ana Lopez", "5216641234567", start)
			booking.Language = test.language
			store.appointments = []reminders.Appointment{booking}
			store.accounts["alice"] = connected("alice")

			sender := &fakeSender{}
			if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if len(sender.sends) != 1 {
				t.Fatalf("sends = %d, want 1", len(sender.sends))
			}
			sent := sender.sends[0]

			if sent.language != test.language {
				t.Errorf("Meta was asked for the %q variant, want %q — the template's own wording comes from this",
					sent.language, test.language)
			}
			if got := sent.parameters[1]; got != test.wantDate {
				t.Errorf("the date reads %q, want %q", got, test.wantDate)
			}
		})
	}
}

// The business setting this replaces held one language for everybody. It is
// still what an appointment with no language of its own falls back to, which
// is what every row written before this column existed looked like until the
// migration copied the setting onto it.
func TestAnAppointmentWithNoLanguageFallsBackToTheBusinessSetting(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	settings := settingsFor("alice", 30*time.Minute)
	settings.DefaultLanguage = "en_US"
	store.settings = []reminders.Settings{settings}
	// Language left at "", which is what a writer that has never heard of
	// the column produces.
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:30:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}
	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1", len(sender.sends))
	}
	if got := sender.sends[0].language; got != "en_US" {
		t.Errorf("Meta was asked for the %q variant, want en_US — the business's own setting", got)
	}
}

// Portuguese is offered by the template form and has no row in the spelling
// table, so an appointment asking for it is a message this app cannot write:
// it would reach Meta as approved Portuguese wording wrapped around a Spanish
// month name. Nothing is claimed and nothing is sent, and the run counts it
// where a person can see it rather than leaving the reminder looking ordinary.
func TestALanguageThisAppCannotSpellADateInIsNotSentAtAll(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
	booking := appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:30:00Z"))
	booking.Language = "pt_BR"
	store.appointments = []reminders.Appointment{booking}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v, want an unwritable reminder to be counted rather than returned", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
	}
	if summary.NoLanguage != 1 {
		t.Errorf("summary = %+v, want the reminder counted as one whose language this app cannot write", summary)
	}
}

// One appointment nobody can write a date for does not stop the rest.
func TestAnUnwritableLanguageDoesNotStopTheRemindersAfterIt(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
	unwritable := appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:30:00Z"))
	unwritable.Language = "pt_BR"
	store.appointments = []reminders.Appointment{
		unwritable,
		appointment("evt2", "alice", "Beto Ruiz", "5216641234568", at(t, "2026-11-03T16:31:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Sent != 1 || summary.NoLanguage != 1 {
		t.Errorf("summary = %+v, want one sent and one counted unwritable", summary)
	}
}

// The row kept for the reminders page records the variant the send actually
// asked Meta for. Recording the business's setting instead would leave a
// clinic reading "es" under a message their patient received in English.
func TestTheStoredRowNamesTheVariantTheSendAskedFor(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
	booking := appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:30:00Z"))
	booking.Language = "en_US"
	store.appointments = []reminders.Appointment{booking}
	store.accounts["alice"] = connected("alice")

	if _, err := reminders.Run(context.Background(), store, &fakeSender{}, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 1 {
		t.Fatalf("claims = %d, want 1", len(store.claims))
	}
	want := "appointment_reminder (en_US): Ana Lopez | November 3, 2026 | 08:30"
	if got := store.claims[0].Content; got != want {
		t.Errorf("the stored row reads %q, want %q", got, want)
	}
}
