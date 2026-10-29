package reminders_test

import (
	"context"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// The window the scheduler scans has to reach as far ahead as the longest
// lead time anybody has configured. The scheduler this replaces used a fixed
// now+25h, which silently capped every lead time at about 1500 minutes: a
// person asking for three days' notice got none at all.
func TestTheWindowReachesAsFarAheadAsTheLongestLeadTime(t *testing.T) {
	now := at(t, "2026-11-03T16:00:00Z")

	store := newStore()
	store.settings = []reminders.Settings{
		settingsFor("alice", 30*time.Minute),
		settingsFor("bob", 72*time.Hour),
	}

	if _, err := reminders.Run(context.Background(), store, &fakeSender{}, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !store.windowFrom.Equal(now) {
		t.Errorf("window starts at %s, want now (%s): an appointment already under way is past being reminded about",
			store.windowFrom, now)
	}
	if want := now.Add(72 * time.Hour); !store.windowTo.Equal(want) {
		t.Errorf("window ends at %s, want %s — the longest configured lead time", store.windowTo, want)
	}
}

// A three-day lead time is the case the old fixed window could not serve, so
// it is worth proving end to end rather than only on the bounds.
func TestALeadTimeBeyondADayStillSends(t *testing.T) {
	now := at(t, "2026-11-03T16:00:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("bob", 72*time.Hour)}
	store.appointments = []reminders.Appointment{
		appointment("evt1", "bob", "Luis", "5216649999999", at(t, "2026-11-06T15:30:00Z")),
	}
	store.accounts["bob"] = connected("bob")

	sender := &fakeSender{}
	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1: a reminder due now for an appointment three days out", len(sender.sends))
	}
}

// Settings and the account lookup are both read once per run. The scheduler
// this replaces read the settings row once per appointment, every minute.
func TestSettingsAndAccountsAreReadOncePerRunNotOncePerAppointment(t *testing.T) {
	now := at(t, "2026-11-03T16:00:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", time.Hour)}
	for _, id := range []string{"evt1", "evt2", "evt3", "evt4"} {
		store.appointments = append(store.appointments,
			appointment(id, "alice", "Ana", "5216641234567", at(t, "2026-11-03T16:30:00Z")))
	}
	store.accounts["alice"] = connected("alice")

	if _, err := reminders.Run(context.Background(), store, &fakeSender{}, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if store.settingsLoads != 1 {
		t.Errorf("settings were loaded %d times, want 1", store.settingsLoads)
	}
	if len(store.accountLookups) != 1 {
		t.Errorf("the account was looked up %d times for one person, want 1: %v",
			len(store.accountLookups), store.accountLookups)
	}
	if len(store.attemptsAsked) != 1 || len(store.attemptsAsked[0]) != 4 {
		t.Errorf("what has already been tried was read as %v, want one query naming all four appointments",
			store.attemptsAsked)
	}
}

// With nobody at all configured there is nothing to scan for.
func TestNobodyConfiguredMeansNoAppointmentQuery(t *testing.T) {
	store := newStore()

	summary, err := reminders.Run(context.Background(), store, &fakeSender{}, at(t, "2026-11-03T16:00:00Z"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !store.windowFrom.IsZero() {
		t.Error("appointments were queried although nobody has reminders switched on")
	}
	if summary.Considered != 0 {
		t.Errorf("summary.Considered = %d, want 0", summary.Considered)
	}
}

// The window is global but the due check is per person: it reaches as far as
// the longest lead time anybody configured, so it necessarily sweeps up
// appointments belonging to people whose own reminder is still hours away.
// Nothing is claimed for those — a claim is a send.
func TestAReminderThatIsNotDueYetIsLeftAlone(t *testing.T) {
	now := at(t, "2026-11-03T16:00:00Z")

	store := newStore()
	store.settings = []reminders.Settings{
		settingsFor("alice", 30*time.Minute),
		// Bob is the reason the window reaches three days out at all.
		settingsFor("bob", 72*time.Hour),
	}
	// Inside the window Bob widened, but Alice wants half an hour of notice
	// and the appointment is a day and a half away.
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana", "5216641234567", at(t, "2026-11-05T04:00:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
	}
	if summary.Considered != 1 || summary.Skipped != 1 {
		t.Errorf("summary = %+v, want one appointment considered and skipped", summary)
	}
}

// The moment a reminder is due is the lead time before the start, to the
// minute. The cron runs on minute boundaries, so the edge is reached exactly.
func TestAReminderDueThisVeryMinuteIsSent(t *testing.T) {
	now := at(t, "2026-11-03T16:00:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana", "5216641234567", at(t, "2026-11-03T16:30:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}
	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1: the reminder is due to the minute", len(sender.sends))
	}
}

// A reminder whose moment passed while the server was down still goes out, as
// long as the appointment itself has not started. The reminders page shows
// exactly these as due.
func TestAReminderWhoseMomentPassedStillGoesOutWhileTheAppointmentIsAhead(t *testing.T) {
	now := at(t, "2026-11-03T16:00:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 24*time.Hour)}
	// Due yesterday afternoon; the appointment is in ten minutes.
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana", "5216641234567", at(t, "2026-11-03T16:10:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	sender := &fakeSender{}
	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1: a late reminder is still worth sending", len(sender.sends))
	}
}

// The date and the time are spelled in the owner's zone, which is the whole
// reason the zone is stored. Tijuana is UTC-7 in July and UTC-8 in November,
// so the same offset cannot be right for both.
func TestTheDateAndTimeFollowTheOwnersZoneAcrossDaylightSaving(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		now        string
		start      string
		wantDate   string
		wantTime   string
		timezone   string
		leadMinute time.Duration
	}{
		{
			name:       "summer, Tijuana is UTC-7",
			now:        "2026-07-15T14:00:00Z",
			start:      "2026-07-15T14:30:00Z",
			wantDate:   "15/07/2026",
			wantTime:   "07:30",
			timezone:   tijuana,
			leadMinute: 30 * time.Minute,
		},
		{
			name:       "winter, Tijuana is UTC-8",
			now:        "2026-11-15T14:00:00Z",
			start:      "2026-11-15T14:30:00Z",
			wantDate:   "15/11/2026",
			wantTime:   "06:30",
			timezone:   tijuana,
			leadMinute: 30 * time.Minute,
		},
		{
			name: "a zone far enough east that the reminder names the next day",
			// 23:30 UTC on the 3rd is 08:30 on the 4th in Tokyo.
			now:        "2026-11-03T23:00:00Z",
			start:      "2026-11-03T23:30:00Z",
			wantDate:   "04/11/2026",
			wantTime:   "08:30",
			timezone:   "Asia/Tokyo",
			leadMinute: 30 * time.Minute,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			settings := settingsFor("alice", testCase.leadMinute)
			settings.Timezone = testCase.timezone

			store := newStore()
			store.settings = []reminders.Settings{settings}
			store.appointments = []reminders.Appointment{
				appointment("evt1", "alice", "Ana", "5216641234567", at(t, testCase.start)),
			}
			store.accounts["alice"] = connected("alice")

			sender := &fakeSender{}
			if _, err := reminders.Run(context.Background(), store, sender, at(t, testCase.now)); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if len(sender.sends) != 1 {
				t.Fatalf("sends = %d, want 1", len(sender.sends))
			}
			got := sender.sends[0].parameters
			if got[1] != testCase.wantDate || got[2] != testCase.wantTime {
				t.Errorf("date/time = %q/%q, want %q/%q", got[1], got[2], testCase.wantDate, testCase.wantTime)
			}
		})
	}
}

// The stored row says which template went out and with what, because the text
// the customer reads is assembled by Meta from a template this app never holds.
func TestTheStoredRowRecordsTheTemplateAndItsParameters(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")

	store := newStore()
	store.settings = []reminders.Settings{settingsFor("alice", 30*time.Minute)}
	store.appointments = []reminders.Appointment{
		appointment("evt1", "alice", "Ana Lopez", "5216641234567", at(t, "2026-11-03T16:30:00Z")),
	}
	store.accounts["alice"] = connected("alice")

	if _, err := reminders.Run(context.Background(), store, &fakeSender{}, now); err != nil {
		t.Fatalf("Run: %v", err)
	}

	claim := store.claims[0]
	const want = "appointment_reminder (es): Ana Lopez | 03/11/2026 | 08:30"
	if claim.Content != want {
		t.Errorf("content = %q, want %q", claim.Content, want)
	}
	if claim.UserID != "alice" || claim.ToNumber != "5216641234567" {
		t.Errorf("claim = %+v, want it to carry the owner and the customer", claim)
	}
	if claim.AccountID != "acct-alice" || claim.PhoneNumberID != "200000000000002" {
		t.Errorf("claim = %+v, want it to record the number it was sent from", claim)
	}
	if !claim.At.Equal(now) {
		t.Errorf("claim.At = %s, want %s", claim.At, now)
	}
}
