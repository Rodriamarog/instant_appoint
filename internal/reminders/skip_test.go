package reminders_test

import (
	"context"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// due builds the one-appointment world every skip case below starts from: a
// reminder that would go out this minute if nothing were wrong with it.
func due(t *testing.T) (*fakeStore, time.Time) {
	t.Helper()

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana", "5216641234567", at(t, "2026-11-03T16:30:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	return store, at(t, "2026-11-03T16:05:00Z")
}

// Somebody who never connected a number, or who disconnected theirs, has no
// way to send at all. The scheduler this replaces was implicitly guarded by
// needing a live browser session; a Cloud API port without this check would
// attempt a send for every such appointment every single minute and fail
// every time.
func TestAnAppointmentWhoseOwnerHasNoConnectedNumberIsSkipped(t *testing.T) {
	store, now := due(t)
	delete(store.accounts, "alice")

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
	}
	if summary.NoAccount != 1 {
		t.Errorf("summary = %+v, want one appointment skipped for having no connected number", summary)
	}
}

// And the lookup that establishes it happens once, not once per appointment,
// so a disconnected account does not cost a query per appointment per minute.
func TestTheMissingAccountIsLookedUpOnceForAllOfThatPersonsAppointments(t *testing.T) {
	store, now := due(t)
	delete(store.accounts, "alice")
	store.appointments = append(store.appointments,
		appointment("evt2", "alice", "Beto", "5216641234568", at(t, "2026-11-03T16:31:00Z")),
		appointment("evt3", "alice", "Cari", "5216641234569", at(t, "2026-11-03T16:32:00Z")),
	)

	if _, err := reminders.Run(context.Background(), store, &fakeSender{}, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.accountLookups) != 1 {
		t.Errorf("the account was looked up %d times, want 1", len(store.accountLookups))
	}
}

// template_name has no default and is empty until somebody picks one. Sending
// a nameless template is a guaranteed rejection from Meta, so an unconfigured
// person is skipped rather than guessed at.
func TestAPersonWhoHasNotChosenATemplateIsSkipped(t *testing.T) {
	store, now := due(t)
	store.settings[0].TemplateName = ""

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
	}
	if summary.NoTemplate != 1 {
		t.Errorf("summary = %+v, want one appointment skipped for having no template chosen", summary)
	}
}

// An appointment with nowhere to send to is not a reminder.
func TestAnAppointmentWithNoClientPhoneIsSkipped(t *testing.T) {
	store, now := due(t)
	store.appointments[0].ClientPhone = ""

	sender := &fakeSender{}
	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
	}
}

// Switching reminders off takes the person out of ActiveSettings entirely, so
// their appointments are swept up by the global window and then dropped.
func TestAppointmentsBelongingToSomebodyWithRemindersOffAreIgnored(t *testing.T) {
	store, now := due(t)
	store.settings = []reminders.Settings{settingsFor("bob", 72*time.Hour)}

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 0 {
		t.Fatalf("sends = %d, want none", len(sender.sends))
	}
	if summary.Considered != 0 {
		t.Errorf("summary.Considered = %d, want 0", summary.Considered)
	}
}

// A reminder already sent is never sent again, and is not even claimed: the
// claim is what the index refuses, and reaching it for a reminder we can
// already see has gone out would mean one needless write per minute.
func TestAReminderAlreadySentIsNotSentAgain(t *testing.T) {
	store, now := due(t)
	store.attempts["evt1"] = reminders.Attempt{Sent: true}

	sender := &fakeSender{}
	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
	}
}

// A zone name the Go runtime cannot resolve would otherwise be silently
// replaced by UTC, which means naming an hour the customer does not recognise.
// Nothing is sent instead.
func TestAnUnresolvableTimezoneStopsTheSendRatherThanFallingBackToUTC(t *testing.T) {
	store, now := due(t)
	store.settings[0].Timezone = "Mars/Olympus_Mons"

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
	}
	if summary.BadTimezone != 1 {
		t.Errorf("summary = %+v, want one appointment skipped for an unusable zone", summary)
	}
}

// One person being unsendable does not stop the run.
func TestOneUnsendablePersonDoesNotStopEverybodyElse(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{
		settingsFor("alice", 30*time.Minute),
		settingsFor("bob", 30*time.Minute),
	}
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana", "5216641234567", at(t, "2026-11-03T16:30:00Z")),
		appointment("evt2", "bob", "Luis", "5216649999999", at(t, "2026-11-03T16:30:00Z")),
	}
	// Only Bob has a connected number.
	store.accounts["bob"] = connected("bob")

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 1 || sender.sends[0].to != "5216649999999" {
		t.Fatalf("sends = %+v, want only the one that could be sent", sender.sends)
	}
	if summary.Sent != 1 || summary.NoAccount != 1 {
		t.Errorf("summary = %+v, want one sent and one skipped", summary)
	}
}
