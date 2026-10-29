package reminderjob

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// PocketBase fires every due tick into a fresh goroutine with no
// serialization — the research measured five concurrent invocations of one
// job. Without a guard a run that outlives its minute would be sending
// alongside the next one, and both would be racing the same claims.
func TestASlowRunIsNeverOverlappedByTheNextTick(t *testing.T) {
	var concurrent, peak, entered atomic.Int64

	release := make(chan struct{})
	inside := make(chan struct{}, 8)

	job := newJob(quietLogger(), time.Minute, func(context.Context) (reminders.Summary, error) {
		entered.Add(1)
		now := concurrent.Add(1)
		for {
			high := peak.Load()
			if now <= high || peak.CompareAndSwap(high, now) {
				break
			}
		}
		inside <- struct{}{}
		<-release
		concurrent.Add(-1)

		return reminders.Summary{}, nil
	})

	// Five ticks at once, which is what was measured in the wild.
	var ticks sync.WaitGroup
	ticks.Add(1)
	go func() {
		defer ticks.Done()
		job()
	}()

	// Wait until the first run is genuinely inside the body before firing the
	// rest, so this tests the guard rather than the scheduler.
	<-inside

	for i := 0; i < 4; i++ {
		ticks.Add(1)
		go func() {
			defer ticks.Done()
			job()
		}()
	}

	// Give the four every chance to get in.
	time.Sleep(50 * time.Millisecond)
	close(release)
	ticks.Wait()

	if peak.Load() != 1 {
		t.Errorf("%d runs were inside the job at once, want 1", peak.Load())
	}
	if entered.Load() != 1 {
		t.Errorf("the job body ran %d times for 5 ticks, want 1", entered.Load())
	}
}

// Once the slow run finishes, the next tick runs normally: the guard skips a
// tick, it does not switch the job off.
func TestTheNextTickAfterASlowRunStillRuns(t *testing.T) {
	var runs atomic.Int64

	job := newJob(quietLogger(), time.Minute, func(context.Context) (reminders.Summary, error) {
		runs.Add(1)
		return reminders.Summary{}, nil
	})

	job()
	job()
	job()

	if runs.Load() != 3 {
		t.Errorf("the job ran %d times for 3 sequential ticks, want 3", runs.Load())
	}
}

// A run that panics must release the guard, or one bad minute would stop
// reminders until the next restart.
func TestAPanickingRunDoesNotWedgeTheGuardForever(t *testing.T) {
	var runs atomic.Int64

	job := newJob(quietLogger(), time.Minute, func(context.Context) (reminders.Summary, error) {
		if runs.Add(1) == 1 {
			panic("the database went away")
		}

		return reminders.Summary{}, nil
	})

	func() {
		defer func() { _ = recover() }()
		job()
	}()

	job()

	if runs.Load() != 2 {
		t.Errorf("the job ran %d times, want 2: the guard was not released after the panic", runs.Load())
	}
}

// Each run is given a deadline. A Graph call that hangs would otherwise hold
// the guard past its minute and stop every later tick.
func TestEachRunIsGivenADeadline(t *testing.T) {
	var deadline time.Time
	var ok bool

	job := newJob(quietLogger(), 50*time.Second, func(ctx context.Context) (reminders.Summary, error) {
		deadline, ok = ctx.Deadline()
		return reminders.Summary{}, nil
	})
	job()

	if !ok {
		t.Fatal("the run was given a context with no deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > 50*time.Second {
		t.Errorf("the deadline is %s away, want within the 50s run timeout", remaining)
	}
}

// The timeout has to leave the next tick a clear minute, or the guard turns a
// slow run into a skipped one every time.
func TestTheRunTimeoutFitsInsideTheMinuteBetweenTicks(t *testing.T) {
	if RunTimeout >= time.Minute {
		t.Errorf("RunTimeout is %s, which does not fit between two ticks a minute apart", RunTimeout)
	}
}

// A failing run is logged and swallowed rather than taking the scheduler down
// with it.
func TestAFailingRunIsSurvived(t *testing.T) {
	job := newJob(quietLogger(), time.Minute, func(context.Context) (reminders.Summary, error) {
		return reminders.Summary{}, errors.New("the database went away")
	})

	job() // must not panic
}
