package reminders_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// The row goes in before Meta is called, never after. If it went after, two
// runs reaching the same minute would both find nothing stored, both send,
// and the customer would be messaged twice — which is the one failure a
// reminder system must not have.
func TestTheRowIsWrittenBeforeMetaIsCalled(t *testing.T) {
	store, now := due(t)
	sender := &fakeSender{}

	// Nothing may have been sent at the moment the claim is written.
	store.onClaim = func() {
		if len(sender.sends) != 0 {
			t.Errorf("a template was sent before the reminder was claimed")
		}
	}

	if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.claims) != 1 || len(sender.sends) != 1 {
		t.Fatalf("claims = %d, sends = %d, want one of each", len(store.claims), len(sender.sends))
	}
}

// A claim the database refuses means another run owns this reminder. That is
// the ordinary outcome of two overlapping runs, not an error, and the only
// correct response is to touch nothing: in particular not to send.
func TestAReminderClaimedByAnotherRunIsSkippedSilentlyAndNeverSent(t *testing.T) {
	store, now := due(t)
	store.alreadyClaimed["evt1"] = true

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v, want a claimed reminder to be an ordinary outcome", err)
	}

	if len(sender.sends) != 0 {
		t.Fatalf("sends = %d, want none: the other run owns this reminder", len(sender.sends))
	}
	if summary.Claimed != 1 || summary.Sent != 0 {
		t.Errorf("summary = %+v, want one reminder recorded as claimed elsewhere", summary)
	}
}

// A claimed reminder does not stop the rest of the run.
func TestAClaimedReminderDoesNotStopTheOnesAfterIt(t *testing.T) {
	store, now := due(t)
	store.alreadyClaimed["evt1"] = true
	store.appointments = append(store.appointments,
		appointment("evt2", "alice", "Beto", "5216641234568", at(t, "2026-11-03T16:31:00Z")))

	sender := &fakeSender{}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Sent != 1 || summary.Claimed != 1 {
		t.Errorf("summary = %+v, want one claimed elsewhere and one sent", summary)
	}
}

// When Meta refuses, the claim is released by turning the row into a failed
// attempt. The unique index is predicated on status='sent', so that is what
// makes the next run able to try again.
func TestAFailedSendReleasesTheClaim(t *testing.T) {
	store, now := due(t)
	sender := &fakeSender{err: errors.New("Recipient phone number not in allowed list")}

	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v, want a refused send to be recorded rather than returned", err)
	}

	if len(store.failed) != 1 || store.failed[0] != "claim-evt1" {
		t.Errorf("failed claims = %v, want the one claim released", store.failed)
	}
	if len(store.sent) != 0 {
		t.Errorf("the reminder was recorded as sent although Meta refused it: %v", store.sent)
	}
	if summary.Failed != 1 || summary.Sent != 0 {
		t.Errorf("summary = %+v, want one failure", summary)
	}
}

// A failure for one person does not stop the run.
func TestAFailedSendDoesNotStopTheOnesAfterIt(t *testing.T) {
	store, now := due(t)
	store.appointments = append(store.appointments,
		appointment("evt2", "alice", "Beto", "5216641234568", at(t, "2026-11-03T16:31:00Z")))

	sender := &failOnce{to: "5216641234567"}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Failed != 1 || summary.Sent != 1 {
		t.Errorf("summary = %+v, want one failure and one send", summary)
	}
}

// failOnce refuses the one recipient it was given and accepts every other.
type failOnce struct {
	to string
}

func (f *failOnce) SendTemplate(_ context.Context, _ reminders.Account, to, _, _ string, _ []string) (string, error) {
	if to == f.to {
		return "", errors.New("Recipient phone number not in allowed list")
	}

	return "wamid." + to, nil
}

// A permanently failing send stops. Without this it would be retried every
// sixty seconds until the appointment began, because a failed attempt
// releases the claim by design.
func TestAPermanentlyFailingSendStopsAfterMaxAttempts(t *testing.T) {
	store, now := due(t)
	store.attempts["evt1"] = reminders.Attempt{
		Failures:      reminders.MaxAttempts,
		LastFailureAt: now.Add(-24 * time.Hour),
	}

	sender := &fakeSender{err: errors.New("Invalid OAuth access token")}
	summary, err := reminders.Run(context.Background(), store, sender, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.claims) != 0 || len(sender.sends) != 0 {
		t.Fatalf("claimed %d and sent %d after %d failures, want neither",
			len(store.claims), len(sender.sends), reminders.MaxAttempts)
	}
	if summary.GaveUp != 1 {
		t.Errorf("summary = %+v, want the reminder given up on", summary)
	}
}

// Walking the whole sequence, which is the property that matters: a send that
// always fails is tried exactly MaxAttempts times across successive runs and
// then never again, rather than forever or only once.
func TestAlwaysFailingIsTriedExactlyMaxAttemptsTimesAcrossRuns(t *testing.T) {
	store, now := due(t)
	sender := &fakeSender{err: errors.New("Invalid OAuth access token")}

	attempt := reminders.Attempt{}
	tries := 0

	// A minute at a time for two hours, which is far longer than the backoff
	// can stretch to within MaxAttempts.
	for minute := 0; minute < 120; minute++ {
		at := now.Add(time.Duration(minute) * time.Minute)
		store.attempts["evt1"] = attempt
		store.claims = nil

		if _, err := reminders.Run(context.Background(), store, sender, at); err != nil {
			t.Fatalf("Run at +%dm: %v", minute, err)
		}

		if len(store.claims) == 1 {
			tries++
			attempt.Failures++
			attempt.LastFailureAt = at
		}
	}

	if tries != reminders.MaxAttempts {
		t.Errorf("the send was tried %d times, want exactly %d", tries, reminders.MaxAttempts)
	}
	if len(sender.sends) != reminders.MaxAttempts {
		t.Errorf("Meta was called %d times, want %d", len(sender.sends), reminders.MaxAttempts)
	}
}

// Between attempts the retry backs off, so a Graph outage is not hammered
// once a minute.
func TestARetryWaitsOutItsBackoff(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		failures int
		since    time.Duration
		wantTry  bool
	}{
		{"one failure, a minute ago", 1, time.Minute, false},
		{"one failure, just short of the backoff", 1, reminders.RetryBackoff - time.Minute, false},
		{"one failure, the backoff elapsed", 1, reminders.RetryBackoff, true},
		{"two failures, the first backoff elapsed", 2, reminders.RetryBackoff, false},
		{"two failures, the doubled backoff elapsed", 2, 2 * reminders.RetryBackoff, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store, now := due(t)
			store.attempts["evt1"] = reminders.Attempt{
				Failures:      testCase.failures,
				LastFailureAt: now.Add(-testCase.since),
			}

			sender := &fakeSender{}
			if _, err := reminders.Run(context.Background(), store, sender, now); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if tried := len(sender.sends) == 1; tried != testCase.wantTry {
				t.Errorf("tried again = %v, want %v", tried, testCase.wantTry)
			}
		})
	}
}

// A run that has outlived its context stops between appointments instead of
// working through a list whose deadline has passed. The cron gives each run a
// timeout precisely so a slow Graph cannot make one run overlap the next.
func TestARunStopsWhenItsContextIsCancelled(t *testing.T) {
	store, now := due(t)
	store.appointments = append(store.appointments,
		appointment("evt2", "alice", "Beto", "5216641234568", at(t, "2026-11-03T16:31:00Z")),
		appointment("evt3", "alice", "Cari", "5216641234569", at(t, "2026-11-03T16:32:00Z")),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.onClaim = cancel

	sender := &fakeSender{}
	summary, err := reminders.Run(ctx, store, sender, now)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}

	if summary.Sent != 1 {
		t.Errorf("summary = %+v, want the one appointment it got to", summary)
	}
	if len(sender.sends) != 1 {
		t.Errorf("sends = %d, want 1: the run should have stopped", len(sender.sends))
	}
}
