package webhook

import (
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/isotime"
)

// Who wrote the appointment is the first thing the receipt turns on, and it
// is the one condition no delivery can exercise: the only caller is handed
// what recordAppointment has just written, which is always the assistant's.
// That is exactly why it is pinned here rather than left to the end-to-end
// cases in receipt_test.go — unreachable today means the rule is held up by
// nothing but the absence of a second caller, and a second caller is a
// receipt sent to a patient about a booking they never discussed with
// anybody. This is an in-package test because that path cannot be reached
// from outside the package at all.
func TestOnlyWhatTheAssistantWroteIsCheckedWithThePatient(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	h := &handler{deps: Deps{Now: func() time.Time { return now }}}

	// No reminders, so the suppression rule cannot be what answers: the
	// writer is the only thing varying across the cases.
	account := Account{Timezone: "America/Tijuana"}

	for _, tc := range []struct {
		name      string
		createdBy appointments.Actor
		want      string
	}{
		{
			name:      "the assistant, out of the conversation the patient just had",
			createdBy: appointments.Assistant,
			want:      "",
		},
		{
			name:      "a member of staff, at the desk or over the phone",
			createdBy: appointments.Staff,
			want:      "the assistant did not write this appointment",
		},
		{
			name:      "nobody recorded",
			createdBy: "",
			want:      "the assistant did not write this appointment",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := h.suppressionReason(account, WhatsApp, "5215511111111", Appointment{
				ClientPhone: "5215511111111",
				StartsAt:    isotime.Format(now.Add(48 * time.Hour)),
				Status:      appointments.Pending,
				CreatedBy:   tc.createdBy,
			})

			if got != tc.want {
				t.Errorf("suppressionReason = %q, want %q", got, tc.want)
			}
		})
	}
}
