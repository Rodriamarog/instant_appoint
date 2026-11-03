package webhook_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// A tap is thrown away today whatever shape it arrives in, so every case here
// is new behaviour. What none of them can prove is the premise underneath:
// that Meta puts the reminder's own wamid in a tap's context. That is settled
// by a live send and nothing else — see the comment on incomingMessage.
// These cases pin what happens on both sides of it, so that the premise being
// wrong surfaces an unmatched tap instead of moving the wrong appointment.

// --- payload builders ----------------------------------------------------

// buttonTap is the shape Meta delivers when a patient taps a quick reply
// under a template: a message of type "button" naming the label, and a
// context pointing at the message the button hung under.
func buttonTap(id, from, label, repliesTo string, sentAt time.Time) map[string]any {
	message := map[string]any{
		"id":        id,
		"from":      from,
		"type":      "button",
		"button":    map[string]any{"payload": label, "text": label},
		"timestamp": strconv.FormatInt(sentAt.Unix(), 10),
	}
	if repliesTo != "" {
		message["context"] = map[string]any{"from": "15550001111", "id": repliesTo}
	}

	return message
}

// --- a tap that lands ----------------------------------------------------

func TestATapOnConfirmConfirmsTheAppointmentItsReminderWasAbout(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

	response := h.post(t, payload([]change{messages(
		buttonTap("wamid.tap-1", "521000000001", "Confirmar", "wamid.reminder-1", frozen),
	)}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "appointment-1", h.store.appointment("appointment-1"), statusChange{
		status: appointments.Confirmed,
		by:     appointments.Patient,
		at:     isotime.Format(frozen),
	})
}

func TestATapOnCancelCancelsTheAppointmentItsReminderWasAbout(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

	response := h.post(t, payload([]change{messages(
		buttonTap("wamid.tap-1", "521000000001", "Cancelar", "wamid.reminder-1", frozen),
	)}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "appointment-1", h.store.appointment("appointment-1"), statusChange{
		status: appointments.Cancelled,
		by:     appointments.Patient,
		at:     isotime.Format(frozen),
	})
}

// The reminder template is approved in one language and a send cannot swap it
// for the recipient's, so a clinic serving English patients has an English
// template and its taps say "Confirm". Matching the Spanish pair alone would
// leave every one of those appointments untouched, silently.
func TestATapIsReadInEveryLanguageAReminderCanGoOutIn(t *testing.T) {
	for _, tc := range []struct {
		label string
		want  appointments.Status
	}{
		{"Confirmar", appointments.Confirmed},
		{"Cancelar", appointments.Cancelled},
		{"Confirm", appointments.Confirmed},
		{"Cancel", appointments.Cancelled},
	} {
		t.Run(tc.label, func(t *testing.T) {
			h := setup(options{})
			h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

			h.post(t, payload([]change{messages(
				buttonTap("wamid.tap-1", "521000000001", tc.label, "wamid.reminder-1", frozen),
			)}))

			assertEqual(t, "status", h.store.appointment("appointment-1").status, tc.want)
		})
	}
}

// The case any "most recent" guess gets wrong. Two reminders are out at once
// for one patient, and the only thing separating them is which of the two the
// tap answers.
func TestATapMovesTheAppointmentItsOwnReminderWasAboutWhenThePatientHasTwo(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-monday", "appointment-monday")
	h.store.sentReminder("user-1", "wamid.reminder-thursday", "appointment-thursday")

	// The patient cancels Monday's, having already been reminded about
	// Thursday's — so the newer reminder is not the one being answered.
	h.post(t, payload([]change{messages(
		buttonTap("wamid.tap-1", "521000000001", "Cancelar", "wamid.reminder-monday", frozen),
	)}))

	assertEqual(t, "monday", h.store.appointment("appointment-monday").status, appointments.Cancelled)
	assertEqual(t, "thursday", h.store.appointment("appointment-thursday").status, appointments.Status(""))
}

// The tap is a turn in the thread like any other, and the row names the
// appointment it moved so the match itself can be checked afterwards.
func TestATapIsStoredInTheConversationLikeAnyOtherMessage(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

	h.post(t, payload([]change{messages(
		buttonTap("wamid.tap-1", "521000000001", "Confirmar", "wamid.reminder-1", frozen),
	)}))

	assertEqual(t, "the stored tap", h.store.message("wamid.tap-1"), webhook.StoredMessage{
		MessageID:      "wamid.tap-1",
		Kind:           webhook.KindTap,
		AppointmentID:  "appointment-1",
		ConversationID: "conv-1",
		CustomerPhone:  "521000000001",
		Content:        "Confirmar",
		Direction:      webhook.Inbound,
		SentAt:         isotime.Format(frozen),
	})
}

// --- a tap that cannot be matched ----------------------------------------

// Each of these is a way the premise can fail. None of them may move an
// appointment, and all of them have to leave the tap where somebody can see
// it: an unmatched row carries no appointment, which is what the thread reads
// to say that nothing happened.
func TestATapThatCannotBeMatchedIsStoredAndChangesNoAppointment(t *testing.T) {
	for _, tc := range []struct {
		name string
		item map[string]any
	}{
		{
			"the delivery named no message it answers",
			buttonTap("wamid.tap-1", "521000000001", "Confirmar", "", frozen),
		},
		{
			"it answers a message that is not a reminder",
			buttonTap("wamid.tap-1", "521000000001", "Confirmar", "wamid.some-chat-reply", frozen),
		},
		{
			"the button is not one this app offers",
			buttonTap("wamid.tap-1", "521000000001", "Reagendar", "wamid.reminder-1", frozen),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := setup(options{})
			h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

			assertStatus(t, h.post(t, payload([]change{messages(tc.item)})), http.StatusOK)

			assertEqual(t, "appointment-1", h.store.appointment("appointment-1"), statusChange{})
			assertEqual(t, "stored", h.store.ids(), []string{"wamid.tap-1"})
			assertEqual(t, "the appointment on the stored tap",
				h.store.message("wamid.tap-1").AppointmentID, "")
		})
	}
}

// A reminder belonging to another clinic is not this clinic's appointment,
// however exactly the wamid matches.
func TestATapIsNotMatchedToAnotherClinicsReminder(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-2", "wamid.reminder-1", "appointment-1")

	h.post(t, payload([]change{messages(
		buttonTap("wamid.tap-1", "521000000001", "Cancelar", "wamid.reminder-1", frozen),
	)}))

	assertEqual(t, "appointment-1", h.store.appointment("appointment-1"), statusChange{})
}

// The status change goes in before the tap does, so that a failure is a
// delivery the router will bring back. Storing the tap first would make the
// retry a duplicate the handler skips, and the patient who cancelled would
// stay booked with nothing anywhere saying why.
func TestAFailedStatusChangeAsksForTheDeliveryAgainAndStoresNothing(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")
	h.store.failSetStatus = errAlways

	response := h.post(t, payload([]change{messages(
		buttonTap("wamid.tap-1", "521000000001", "Cancelar", "wamid.reminder-1", frozen),
	)}))

	assertStatus(t, response, http.StatusInternalServerError)
	assertEqual(t, "stored", h.store.ids(), []string{})
}

// --- what a tap must not disturb -----------------------------------------

// A tap is an answer, not a question. The patient pressed one of two buttons
// under a message they have already read; an assistant replying would be a
// second conversation nobody asked for, about a cancellation it did not make.
func TestATapIsNotAnsweredByTheAssistant(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

	h.post(t, payload([]change{messages(
		buttonTap("wamid.tap-1", "521000000001", "Confirmar", "wamid.reminder-1", frozen),
	)}))

	assertEqual(t, "sends", h.sends(), []send{})
}

// The words a patient types are still the words a patient types, context or
// no context. Quoting a reminder and writing "Confirmar" is a sentence for
// the assistant to read, not a button press that cancels a booking.
func TestATypedReplyQuotingAReminderIsStillAnOrdinaryMessage(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

	quoted := textMessage("wamid.typed", "521000000001", "Confirmar", frozen)
	quoted["context"] = map[string]any{"id": "wamid.reminder-1"}

	h.post(t, payload([]change{messages(quoted)}))

	assertEqual(t, "appointment-1", h.store.appointment("appointment-1"), statusChange{})
	assertEqual(t, "the stored kind", h.store.message("wamid.typed").Kind, webhook.KindText)
	assertEqual(t, "sentTo", h.sentTo(), []string{"521000000001"})
}

// The other shape a tap can arrive in. This app does not send interactive
// messages today, so nothing produces it — it is read anyway because if a
// reminder is ever sent that way, the difference should be that nothing here
// had to change rather than that every tap went quiet.
func TestAnInteractiveButtonReplyIsReadAsATapToo(t *testing.T) {
	h := setup(options{})
	h.store.sentReminder("user-1", "wamid.reminder-1", "appointment-1")

	h.post(t, payload([]change{messages(map[string]any{
		"id":        "wamid.tap-1",
		"from":      "521000000001",
		"type":      "interactive",
		"timestamp": strconv.FormatInt(frozen.Unix(), 10),
		"context":   map[string]any{"id": "wamid.reminder-1"},
		"interactive": map[string]any{
			"type":         "button_reply",
			"button_reply": map[string]any{"id": "btn-0", "title": "Confirmar"},
		},
	})}))

	assertEqual(t, "status", h.store.appointment("appointment-1").status, appointments.Confirmed)
	assertEqual(t, "content", h.store.message("wamid.tap-1").Content, "Confirmar")
}
