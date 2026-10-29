package reminderjob_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/reminderjob"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// These run against a real PocketBase, because what suppresses the reminder
// is a real partial unique index. A fake cannot refuse the second insert, so
// a fake cannot show the bug or the fix.

// abandonedClaim writes the row a process killed mid-send leaves behind:
// status='sent', because the claim is written before Meta is called, and no
// message_id, because Meta never answered.
func abandonedClaim(t *testing.T, app core.App, userID, eventID string, at time.Time) string {
	t.Helper()

	return save(t, app, "whatsapp_messages", map[string]any{
		"user_id": userID, "to_number": "5216641234567", "event_id": eventID,
		"message_type": "reminder", "direction": "outbound",
		"status": "sent", "sent_at": iso(at),
	})
}

// A claim nothing finished reads as sent, and the index refuses every later
// one, so before this fix a single crash meant the reminder was never sent
// and never could be.
func TestAClaimLeftUnfinishedIsReclaimedOnceItIsCertainlyDead(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")
	accountID := connectNumber(t, app, alice)
	store := reminderjob.NewStore(app)

	crashed := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)
	dead := abandonedClaim(t, app, alice, "evt1", crashed)

	now := crashed.Add(reminders.ReclaimAfter)

	// The run has to get as far as claiming at all: an attempt that still
	// reads as sent is skipped long before Claim is reached.
	attempts, err := store.ReminderAttempts(context.Background(), []string{"evt1"})
	if err != nil {
		t.Fatalf("ReminderAttempts: %v", err)
	}
	if !attempts["evt1"].Abandoned(now) {
		t.Fatalf("the unfinished claim reads as %+v, which does not let the run reach Claim", attempts["evt1"])
	}

	claimID, err := store.Claim(context.Background(), claimFor("evt1", alice, accountID, now))
	if err != nil {
		t.Fatalf("Claim: %v, want the dead claim taken over", err)
	}
	if claimID == dead {
		t.Error("the dead row was reused rather than replaced, which would lose the record of the crash")
	}

	// The crash is still in the audit trail, as a failed attempt — which is
	// also what bounds the retry if it keeps happening.
	rows := reminderRows(t, app, "evt1")
	if len(rows) != 2 {
		t.Fatalf("there are %d reminder rows, want the dead one plus the new claim", len(rows))
	}

	byID := map[string]string{}
	for _, row := range rows {
		byID[row.Id] = row.GetString("status")
	}
	if byID[dead] != "failed" {
		t.Errorf("the dead claim is now %q, want failed so the index releases it", byID[dead])
	}
	if byID[claimID] != "sent" {
		t.Errorf("the new claim is %q, want sent", byID[claimID])
	}
}

// Everything short of certainly-dead is left alone.
func TestAClaimIsNotReclaimedWhileItCouldStillBeAlive(t *testing.T) {
	claimed := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)

	cases := []struct {
		name      string
		messageID string
		now       time.Time
	}{
		{"a send that is merely slow", "", claimed.Add(time.Minute)},
		{"one moment short of the window", "", claimed.Add(reminders.ReclaimAfter - time.Millisecond)},
		{"long past the window, but it did send", "wamid.ABC", claimed.Add(24 * time.Hour)},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			app := newApp(t)
			alice := newUser(t, app, "alice@instantappoint.test")
			accountID := connectNumber(t, app, alice)
			store := reminderjob.NewStore(app)

			held := abandonedClaim(t, app, alice, "evt1", claimed)
			if test.messageID != "" {
				record, err := app.FindRecordById("whatsapp_messages", held)
				if err != nil {
					t.Fatalf("read the claim back: %v", err)
				}
				record.Set("message_id", test.messageID)
				if err := app.Save(record); err != nil {
					t.Fatalf("record the message id: %v", err)
				}
			}

			_, err := store.Claim(context.Background(), claimFor("evt1", alice, accountID, test.now))
			if !errors.Is(err, reminders.ErrAlreadyClaimed) {
				t.Fatalf("Claim returned %v, want ErrAlreadyClaimed — this claim is not dead", err)
			}

			rows := reminderRows(t, app, "evt1")
			if len(rows) != 1 {
				t.Fatalf("there are %d reminder rows, want the one that was already there", len(rows))
			}
			if got := rows[0].GetString("status"); got != "sent" {
				t.Errorf("the existing claim is now %q, want it untouched at sent", got)
			}
		})
	}
}

// What the run reads has to show the difference too, or it skips the
// appointment before Claim is ever called.
func TestReminderAttemptsTellsAnUnfinishedClaimFromAFinishedOne(t *testing.T) {
	app := newApp(t)
	alice := newUser(t, app, "alice@instantappoint.test")

	claimed := time.Date(2026, 11, 3, 16, 5, 0, 0, time.UTC)
	abandonedClaim(t, app, alice, "evt-unfinished", claimed)

	finished := abandonedClaim(t, app, alice, "evt-finished", claimed)
	record, err := app.FindRecordById("whatsapp_messages", finished)
	if err != nil {
		t.Fatalf("read the claim back: %v", err)
	}
	record.Set("message_id", "wamid.ABC")
	if err := app.Save(record); err != nil {
		t.Fatalf("record the message id: %v", err)
	}

	attempts, err := reminderjob.NewStore(app).ReminderAttempts(context.Background(),
		[]string{"evt-unfinished", "evt-finished"})
	if err != nil {
		t.Fatalf("ReminderAttempts: %v", err)
	}

	unfinished := attempts["evt-unfinished"]
	if !unfinished.Sent || !unfinished.Incomplete {
		t.Errorf("the unfinished claim reads as %+v, want sent and incomplete", unfinished)
	}
	if !unfinished.ClaimedAt.Equal(claimed) {
		t.Errorf("ClaimedAt = %s, want the moment the claim was written, %s", unfinished.ClaimedAt, claimed)
	}

	if done := attempts["evt-finished"]; !done.Sent || done.Incomplete {
		t.Errorf("a reminder with a message id reads as %+v, want sent and complete", done)
	}
}
