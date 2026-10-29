// Package reminderjob is the PocketBase half of the reminder scheduler.
//
// internal/reminders decides which reminders are due and imports nothing from
// PocketBase, so that its behaviour is testable with no database. Everything
// that knows about collections, records, SQLite and the Graph API lives here,
// on the other side of those interfaces, together with the cron registration.
package reminderjob

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// JobID is the cron job's name. PocketBase replaces a job registered twice
// under one id rather than running both, so re-registration is safe.
//
// The ids beginning and ending in double underscores are PocketBase's own
// (__pbDBOptimize__ and friends); this one deliberately is not.
const JobID = "reminders"

// Schedule is how often the scheduler looks for reminders that have fallen
// due. A minute is the finest granularity PocketBase's cron offers — its
// expressions have no seconds field — and it is also what the lead times the
// settings dialog offers are rounded to.
const Schedule = "* * * * *"

// RunTimeout bounds one run.
//
// It has to fit inside the minute between two ticks: the overlap guard turns
// a run that outlives its minute into a skipped tick, so a hung Graph call
// without this deadline would skip every tick until the server restarted. Ten
// seconds of headroom under the minute is enough for the run to unwind and
// log.
const RunTimeout = 50 * time.Second

// Register schedules the reminder run on app's cron.
//
// PocketBase starts its cron from an OnServe hook, so nothing is scheduled
// under `migrate` or any other command — only under `serve`.
func Register(app core.App, client *meta.Client) {
	store := NewStore(app)
	sender := NewSender(client)

	app.Cron().MustAdd(JobID, Schedule, newJob(app.Logger(), RunTimeout,
		func(ctx context.Context) (reminders.Summary, error) {
			return reminders.Run(ctx, store, sender, time.Now())
		}))
}

// newJob wraps one run in the two things the cron does not provide: a guard
// against overlapping itself, and a deadline.
//
// PocketBase fires every due tick into a fresh goroutine with no
// serialization — five concurrent invocations of one job were measured
// directly — so without the guard a run that outlives its minute would be
// claiming and sending alongside the next one. The claim's unique index would
// still stop a double send; this stops the pile-up that would be racing it.
func newJob(logger *slog.Logger, timeout time.Duration, run func(context.Context) (reminders.Summary, error)) func() {
	// Captured per registration rather than package-level, so a second
	// registration — in a test, or a second app in one process — gets its own
	// guard rather than silently sharing this one.
	var running atomic.Bool

	return func() {
		if !running.CompareAndSwap(false, true) {
			logger.Warn("reminders: the previous run is still going, skipping this minute")
			return
		}
		// Released even if the run panics: PocketBase contains a panic inside
		// a cron job, so without this one bad minute would stop reminders
		// until the next restart rather than for one tick.
		defer running.Store(false)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		summary, err := run(ctx)
		if err != nil {
			// Logged, not returned: there is nobody to return it to, and the
			// next tick is a minute away. What was done before the failure
			// still stands.
			logger.Error("reminders: the run did not finish", "error", err, "summary", summary)
			return
		}

		if summary.Considered == 0 {
			return
		}

		logger.Info("reminders: run finished",
			"considered", summary.Considered,
			"sent", summary.Sent,
			"failed", summary.Failed,
			"claimed_elsewhere", summary.Claimed,
			"no_connected_number", summary.NoAccount,
			"no_template_chosen", summary.NoTemplate,
			"unusable_timezone", summary.BadTimezone,
			"given_up", summary.GaveUp,
			"skipped", summary.Skipped,
		)
	}
}
