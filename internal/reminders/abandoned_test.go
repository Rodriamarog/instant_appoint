package reminders_test

import (
	"context"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// A claim is written as status='sent' before Meta is called, so a process that
// dies in between leaves a row that says a reminder went out when none did —
// and the unique index then refuses every later claim for that appointment.
// Without the reclaim below, one crash suppresses one reminder forever, with
// nothing sent and nothing logged.
//
// The cost is stated where ReclaimAfter is declared: a send that did reach
// Meta before the crash goes out twice. For a reminder that is the better
// half of a tradeoff that has no third option.
func TestAClaimLeftUnfinishedByACrashIsEventuallyTriedAgain(t *testing.T) {
	store, now := due(t)
	store.attempts["evt1"] = reminders.Attempt{
		Sent:       true,
		Incomplete: true,
		ClaimedAt:  now.Add(-reminders.ReclaimAfter),
	}

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(sender.sends) != 1 {
		t.Fatalf("sends = %d, want 1 — a reminder nothing ever sent must not stay suppressed", len(sender.sends))
	}
	if summary.Sent != 1 {
		t.Errorf("summary = %+v, want the reminder recorded as sent", summary)
	}
}

// Everything that makes the row look dead has to be true at once. A claim a
// live run is still working through, and a claim that really did go out, are
// both left exactly where they are.
func TestAClaimIsNotTakenOverWhileItCouldStillBeAlive(t *testing.T) {
	cases := []struct {
		name    string
		attempt func(now time.Time) reminders.Attempt
	}{
		{
			// The run deadline is 50s and the Graph client times out after
			// 10s, so a claim one minute old is at worst a slow send.
			"a send that is merely slow",
			func(now time.Time) reminders.Attempt {
				return reminders.Attempt{Sent: true, Incomplete: true, ClaimedAt: now.Add(-time.Minute)}
			},
		},
		{
			"a claim one moment short of the window",
			func(now time.Time) reminders.Attempt {
				return reminders.Attempt{Sent: true, Incomplete: true, ClaimedAt: now.Add(-reminders.ReclaimAfter + time.Second)}
			},
		},
		{
			// A message id means Meta answered: the reminder went out, and
			// however old the row is it is finished.
			"an old reminder that really was sent",
			func(now time.Time) reminders.Attempt {
				return reminders.Attempt{Sent: true, ClaimedAt: now.Add(-30 * 24 * time.Hour)}
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			store, now := due(t)
			store.attempts["evt1"] = test.attempt(now)

			sender := &fakeSender{}
			summary, err := reminders.Run(context.Background(), store, sender, now)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			if len(store.claims) != 0 || len(sender.sends) != 0 {
				t.Fatalf("claimed %d and sent %d, want neither", len(store.claims), len(sender.sends))
			}
			if summary.Skipped != 1 {
				t.Errorf("summary = %+v, want the reminder skipped", summary)
			}
		})
	}
}

// Abandoned is the whole rule, so it is worth stating on its own.
func TestAbandonedNeedsASentRowWithNoMessageIdOldEnoughToBeDead(t *testing.T) {
	now := at(t, "2026-11-03T16:05:00Z")
	old := now.Add(-reminders.ReclaimAfter)

	cases := []struct {
		name    string
		attempt reminders.Attempt
		want    bool
	}{
		{"an old unfinished claim", reminders.Attempt{Sent: true, Incomplete: true, ClaimedAt: old}, true},
		{"older still", reminders.Attempt{Sent: true, Incomplete: true, ClaimedAt: old.Add(-time.Hour)}, true},
		{"not yet old enough", reminders.Attempt{Sent: true, Incomplete: true, ClaimedAt: old.Add(time.Millisecond)}, false},
		{"finished, so not abandoned", reminders.Attempt{Sent: true, ClaimedAt: old}, false},
		{"nothing sent at all", reminders.Attempt{}, false},
		{"only failures", reminders.Attempt{Failures: 2, LastFailureAt: old}, false},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := test.attempt.Abandoned(now); got != test.want {
				t.Errorf("Abandoned(%s) = %v, want %v for %+v", now, got, test.want, test.attempt)
			}
		})
	}
}

// The window has to be long enough that nothing healthy is ever mistaken for
// something dead. Shrinking it towards the run deadline would make a slow
// send look abandoned while it is still in flight, which is how a duplicate
// stops being rare.
func TestTheReclaimWindowIsFarLongerThanARunCanTake(t *testing.T) {
	const runDeadline = 50 * time.Second

	if reminders.ReclaimAfter < 10*runDeadline {
		t.Errorf("ReclaimAfter = %s, want at least ten times the %s run deadline",
			reminders.ReclaimAfter, runDeadline)
	}
}
