package reminders_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// The whole point of internal/reminders is that it can be run without a
// database, a network or a clock. These fakes are what that buys: every case
// below is a few lines of setup and an assertion about what was sent.

// fakeStore records what Run asked for and what it wrote.
type fakeStore struct {
	settings     []reminders.Settings
	appointments []reminders.Appointment
	accounts     map[string]*reminders.Account
	attempts     map[string]reminders.Attempt

	// What Run asked of it.
	settingsLoads  int
	accountLookups []string
	windowFrom     time.Time
	windowTo       time.Time
	attemptsAsked  [][]string

	// What Run wrote.
	claims []reminders.Claim
	sent   map[string]string
	failed []string

	// Behaviour to provoke.
	alreadyClaimed map[string]bool
	claimErr       error
	settingsErr    error
	appointmentErr error

	// onClaim runs before each claim is recorded, so a test can make the
	// world change mid-run.
	onClaim func()
}

func newStore() *fakeStore {
	return &fakeStore{
		accounts:       map[string]*reminders.Account{},
		attempts:       map[string]reminders.Attempt{},
		sent:           map[string]string{},
		alreadyClaimed: map[string]bool{},
	}
}

func (s *fakeStore) ActiveSettings(context.Context) ([]reminders.Settings, error) {
	s.settingsLoads++
	return s.settings, s.settingsErr
}

func (s *fakeStore) AppointmentsStartingBetween(_ context.Context, from, to time.Time) ([]reminders.Appointment, error) {
	s.windowFrom, s.windowTo = from, to
	if s.appointmentErr != nil {
		return nil, s.appointmentErr
	}

	var inWindow []reminders.Appointment
	for _, appointment := range s.appointments {
		if !appointment.StartTime.Before(from) && !appointment.StartTime.After(to) {
			inWindow = append(inWindow, appointment)
		}
	}

	return inWindow, nil
}

func (s *fakeStore) AccountFor(_ context.Context, userID string) (*reminders.Account, error) {
	s.accountLookups = append(s.accountLookups, userID)
	return s.accounts[userID], nil
}

func (s *fakeStore) ReminderAttempts(_ context.Context, ids []string) (map[string]reminders.Attempt, error) {
	s.attemptsAsked = append(s.attemptsAsked, ids)

	asked := map[string]reminders.Attempt{}
	for _, id := range ids {
		if attempt, ok := s.attempts[id]; ok {
			asked[id] = attempt
		}
	}

	return asked, nil
}

func (s *fakeStore) Claim(_ context.Context, claim reminders.Claim) (string, error) {
	if s.claimErr != nil {
		return "", s.claimErr
	}
	if s.onClaim != nil {
		s.onClaim()
	}
	if s.alreadyClaimed[claim.AppointmentID] {
		return "", reminders.ErrAlreadyClaimed
	}

	s.claims = append(s.claims, claim)

	return fmt.Sprintf("claim-%s", claim.AppointmentID), nil
}

func (s *fakeStore) MarkSent(_ context.Context, claimID, messageID string) error {
	s.sent[claimID] = messageID
	return nil
}

func (s *fakeStore) MarkFailed(_ context.Context, claimID string) error {
	s.failed = append(s.failed, claimID)
	return nil
}

// fakeSender records every template it was asked to send.
type fakeSender struct {
	sends []send
	err   error
}

type send struct {
	account    reminders.Account
	to         string
	template   string
	language   string
	parameters []string
}

func (s *fakeSender) SendTemplate(_ context.Context, account reminders.Account, to, name, language string, parameters []string) (string, error) {
	s.sends = append(s.sends, send{account, to, name, language, parameters})
	if s.err != nil {
		return "", s.err
	}

	return "wamid." + to, nil
}

// at reads a wall-clock time in UTC. Every test below fixes "now" this way, so
// nothing depends on when it runs.
func at(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}

	return parsed.UTC()
}
