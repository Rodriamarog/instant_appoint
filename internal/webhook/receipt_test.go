package webhook_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// The receipt is the safety net under everything the assistant writes. It
// puts the appointment the model read out of a conversation in front of the
// one person who knows for certain whether they agreed to it, while the
// conversation is still in front of them — which is the whole difference
// from the reminder, arriving a day later with the morning already planned
// around a booking nobody made.
func TestAnAppointmentTheAssistantWritesIsSentBackToThePatientToCheck(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	response := h.post(t, payload([]change{messages(textMessage("wamid.in", "5215511111111", "el viernes a las 3", frozen))}))

	assertStatus(t, response, http.StatusOK)

	receipts := h.receipts()
	if len(receipts) != 1 {
		t.Fatalf("sent %d receipts, want one: %#v", len(receipts), receipts)
	}

	// 2026-10-30T22:00:00Z is 15:00 in Tijuana, which is the clock the
	// clinic and the patient both keep. Naming the UTC hour would send
	// somebody in at ten at night.
	want := "Tu cita quedó registrada para el 30 de octubre de 2026 a las 15:00. ¿Es correcto?"
	if receipts[0].body != want {
		t.Errorf("the receipt reads  %q\nwant               %q", receipts[0].body, want)
	}
	if receipts[0].to != "5215511111111" {
		t.Errorf("the receipt went to %q, want the patient", receipts[0].to)
	}

	// The two buttons, in the order the tap resolver reads position from.
	assertEqual(t, "the buttons", receipts[0].buttons, []webhook.Button{
		{ID: "confirm", Title: "Confirmar"},
		{ID: "cancel", Title: "Cancelar"},
	})
}

// findsAnAppointmentAt is a model that reads one specific agreement out of
// any conversation, for the cases that turn on when the appointment is.
func findsAnAppointmentAt(start time.Time, certain bool) func(context.Context, []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
	return func(context.Context, []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
		return &webhook.ExtractedAppointment{
			ClientName: "Ana Ruiz",
			StartsAt:   start,
			Certain:    certain,
			Summary:    "Limpieza dental",
		}, nil
	}
}

// The suppression rule, and the reason this ticket has one.
//
// An appointment booked inside its own lead time has its reminder fall due
// the moment it exists. Without this the patient gets a receipt and then,
// seconds later, a reminder saying very nearly the same thing and offering
// the same two buttons — and has no way to tell whether the clinic has
// booked them twice. When the reminder is already due, the reminder is the
// receipt.
func TestNoReceiptIsSentWhenTheAppointmentsOwnReminderIsAlreadyDue(t *testing.T) {
	// Eight hours away, with the clinic's day of lead time: the reminder
	// for this one fell due before it was written.
	h := setup(options{extract: findsAnAppointmentAt(frozen.Add(8*time.Hour), true)})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{messages(textMessage("wamid.today", "5215511111111", "¿hoy a las 8?", frozen))}))

	if got := len(h.store.written()); got != 1 {
		t.Fatalf("wrote %d appointments, want the appointment itself kept", got)
	}
	assertEqual(t, "receipts", h.receipts(), []receipt{})
}

// A clinic that sends no reminders at all has no second message to defer
// to, so the receipt is the only thing the patient will ever get. Reading
// "the reminder is already due" off a reminder that is never sent would
// leave a same-day booking confirmed by nobody.
func TestAClinicThatSendsNoRemindersStillSendsTheReceipt(t *testing.T) {
	h := setup(options{extract: findsAnAppointmentAt(frozen.Add(8*time.Hour), true)})
	h.store.setMode(webhook.ModeObserver)
	h.store.setAccount(func(account *webhook.Account) { account.SendsReminders = false })

	h.post(t, payload([]change{messages(textMessage("wamid.today", "5215511111111", "¿hoy a las 8?", frozen))}))

	if got := len(h.receipts()); got != 1 {
		t.Errorf("sent %d receipts, want one", got)
	}
}

// --- whether the assistant may message a patient at all (#36) ------------

// The control, and the only thing it is for. A clinic that has said the
// assistant may not write to its patients unprompted means every
// appointment, not only the ones the other suppression rules already catch
// — so this is asserted in both modes that write appointments rather than
// in the one the rest of this file happens to use.
func TestAClinicThatHasTurnedTheReceiptOffGetsNoneInAnyMode(t *testing.T) {
	for _, mode := range []webhook.Mode{webhook.ModeObserver, webhook.ModeAutopilot} {
		t.Run(string(mode), func(t *testing.T) {
			h := setup(options{extract: findsAnAppointment(true)})
			h.store.setMode(mode)
			h.store.setOutreach(webhook.OutreachOff)

			h.post(t, payload([]change{messages(textMessage("wamid.in", "5215511111111", "el viernes a las 3", frozen))}))

			// The appointment is still written. This control is about who
			// the assistant talks to, not about what it may put in the
			// calendar — that decision is the mode's, and the clinic has
			// already made it.
			if got := len(h.store.written()); got != 1 {
				t.Fatalf("wrote %d appointments, want the appointment itself kept", got)
			}
			assertEqual(t, "receipts", h.receipts(), []receipt{})
		})
	}
}

// Autopilot is answering somebody who just wrote in, which is not the
// assistant speaking unprompted and is not this control's business. A
// clinic that wanted its patients unanswered has a switch for that, and it
// is the mode.
func TestAutopilotStillAnswersAPatientWhoWroteInWhileOutreachIsOff(t *testing.T) {
	h := setup(options{})
	h.store.setMode(webhook.ModeAutopilot)
	h.store.setOutreach(webhook.OutreachOff)

	response := h.post(t, payload([]change{messages(textMessage("wamid.auto", "5215511111111", "hola", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "sent", h.sends(), []send{
		{phoneNumberID: phoneNumberID, to: "5215511111111", text: "reply to: hola"},
	})
}

// A proposal is not an agreement. It exists because the conversation was
// hedged or partial, and it is written where a member of staff will look at
// it — asking the patient to confirm a time the clinic has not offered them
// is how a phantom appointment becomes a real one.
func TestAProposalGetsNoReceipt(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(false)})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{messages(textMessage("wamid.maybe", "5215511111111", "igual el martes", frozen))}))

	written := h.store.written()
	if len(written) != 1 || written[0].Status != appointments.Proposed {
		t.Fatalf("wrote %#v, want one proposal", written)
	}
	assertEqual(t, "receipts", h.receipts(), []receipt{})
}

// receiptTap is the shape Meta delivers when a patient taps one of a
// receipt's reply buttons: an interactive message carrying the id this app
// set on the button and the title it showed, and a context naming the
// receipt itself.
func receiptTap(id, from, buttonID, title, repliesTo string) map[string]any {
	return map[string]any{
		"id":        id,
		"from":      from,
		"type":      "interactive",
		"timestamp": strconv.FormatInt(frozen.Unix(), 10),
		"context":   map[string]any{"id": repliesTo},
		"interactive": map[string]any{
			"type":         "button_reply",
			"button_reply": map[string]any{"id": buttonID, "title": title},
		},
	}
}

// The whole point of the receipt, end to end: the patient reads what the
// assistant wrote down and answers it, and the answer reaches the
// appointment.
//
// It works only because the receipt's own send is recorded as a row
// carrying both the appointment and Meta's id for the message — the same
// join a reminder's taps go through. A receipt sent without that row would
// look entirely correct from the outside and every tap on it would arrive
// matching nothing.
func TestATapOnAReceiptMovesTheAppointmentItIsAbout(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{messages(textMessage("wamid.in", "5215511111111", "el viernes a las 3", frozen))}))

	written := h.store.written()
	if len(written) != 1 || len(h.receipts()) != 1 {
		t.Fatalf("wrote %d appointments and sent %d receipts, want one of each", len(written), len(h.receipts()))
	}

	// The id the fake handed back for the receipt, which is what Meta puts
	// in the context of a tap on it.
	stored := h.store.message("wamid.receipt-1")
	if stored.AppointmentID == "" {
		t.Fatalf("the receipt was stored as %#v, want a row naming the appointment it is about", stored)
	}

	h.post(t, payload([]change{messages(
		receiptTap("wamid.tap-1", "5215511111111", "confirm", "Confirmar", "wamid.receipt-1"),
	)}))

	assertEqual(t, "the appointment", h.store.appointment(stored.AppointmentID), statusChange{
		status: appointments.Confirmed,
		by:     appointments.Patient,
		at:     isotime.Format(frozen),
	})
}

// Meta hands a reply button's id back untouched, and it is the one part of
// a tap that says the same thing whatever language the patient reads. A
// delivery that carried only the id — the title dropped, or a shape this
// app has not seen live — still has to resolve, because the alternative is
// an English patient's cancellation silently doing nothing.
func TestAReceiptsButtonIsRecognisedByItsIdAloneWhateverTheTitleSays(t *testing.T) {
	for _, tc := range []struct {
		buttonID string
		want     appointments.Status
	}{
		{"confirm", appointments.Confirmed},
		{"cancel", appointments.Cancelled},
	} {
		h := setup(options{})
		h.store.sentReminder("user-1", "wamid.receipt-1", "appointment-1")

		h.post(t, payload([]change{messages(
			receiptTap("wamid.tap-"+tc.buttonID, "5215511111111", tc.buttonID, "", "wamid.receipt-1"),
		)}))

		assertEqual(t, "the status after "+tc.buttonID, h.store.appointment("appointment-1").status, tc.want)
	}
}

// The appointment is the valuable thing and the message is best-effort. A
// Meta outage at the wrong second must not cost the clinic the booking the
// assistant just read out of a conversation, nor have the router redeliver
// a message that is already stored.
func TestAReceiptThatCannotBeSentKeepsTheAppointmentAnyway(t *testing.T) {
	h := setup(options{
		extract: findsAnAppointment(true),
		sendButtons: func(context.Context, string, string, string, string, []webhook.Button) (string, error) {
			return "", errors.New("the Graph API is unreachable")
		},
	})
	h.store.setMode(webhook.ModeObserver)

	response := h.post(t, payload([]change{messages(textMessage("wamid.in", "5215511111111", "el viernes a las 3", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "stored", h.store.ids(), []string{"wamid.in"})
	if got := len(h.store.written()); got != 1 {
		t.Errorf("wrote %d appointments, want the appointment kept", got)
	}
}

// The duplicate rule as the database settles it. Two deliveries of one
// conversation can be in flight at once, and the one whose write the unique
// index refuses wrote no appointment — so it has nothing to tell the
// patient about. The delivery that won has already told them, and a second
// receipt is the duplicate message this whole feature exists to avoid.
func TestADeliveryThatLostTheRaceToWriteSendsNoReceipt(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)
	h.store.refuseCreateAppointment = true

	response := h.post(t, payload([]change{messages(textMessage("wamid.in", "5215511111111", "el viernes a las 3", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "receipts", h.receipts(), []receipt{})
}

// One appointment, one receipt. The conversation carries on after the
// agreement, as every real one does, and the duplicate rule stops a second
// appointment being written — so there is nothing for a second receipt to
// be about.
func TestAConversationThatKeepsGoingSendsOnlyOneReceipt(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	for _, text := range []string{"el viernes a las 3", "perfecto", "gracias"} {
		h.post(t, payload([]change{messages(textMessage("wamid."+text, "5215511111111", text, frozen))}))
	}

	if got := len(h.receipts()); got != 1 {
		t.Errorf("sent %d receipts for one appointment, want one", got)
	}
}

// A Tijuana clinic's patients are on both sides of the border, and the one
// fact the conversation always carries is the number it came from. Writing
// the appointment without a language leaves it on the column's Spanish
// default, so a thread held entirely in English would produce a Spanish
// receipt and, a day later, a Spanish reminder.
func TestThePatientsOwnLanguageIsWrittenOnTheAppointmentAndUsedForTheReceipt(t *testing.T) {
	for _, tc := range []struct {
		name         string
		from         string
		wantLanguage string
		wantBody     string
	}{
		{
			name:         "a Mexican number",
			from:         "5216641234567",
			wantLanguage: "es",
			wantBody:     "Tu cita quedó registrada para el 30 de octubre de 2026 a las 15:00. ¿Es correcto?",
		},
		{
			name:         "a US number",
			from:         "16191234567",
			wantLanguage: "en_US",
			wantBody:     "Your appointment is booked for October 30, 2026 at 15:00. Is that right?",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := setup(options{extract: findsAnAppointment(true)})
			h.store.setMode(webhook.ModeObserver)

			h.post(t, payload([]change{messages(textMessage("wamid.in", tc.from, "el viernes a las 3", frozen))}))

			written := h.store.written()
			if len(written) != 1 {
				t.Fatalf("wrote %d appointments, want one", len(written))
			}
			if written[0].Language != tc.wantLanguage {
				t.Errorf("the appointment's language is %q, want %q", written[0].Language, tc.wantLanguage)
			}

			receipts := h.receipts()
			if len(receipts) != 1 {
				t.Fatalf("sent %d receipts, want one", len(receipts))
			}
			if receipts[0].body != tc.wantBody {
				t.Errorf("the receipt reads  %q\nwant               %q", receipts[0].body, tc.wantBody)
			}
		})
	}
}

// The titles are what the patient reads, so they follow the appointment's
// language too — and both of them have to stay in the table that resolves a
// tap, or an English patient's answer arrives naming a button this app
// cannot place.
func TestAnEnglishReceiptsButtonsAreReadBackAsTheSameTwoActions(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{messages(textMessage("wamid.in", "16191234567", "friday at three", frozen))}))

	receipts := h.receipts()
	if len(receipts) != 1 {
		t.Fatalf("sent %d receipts, want one", len(receipts))
	}
	assertEqual(t, "the buttons", receipts[0].buttons, []webhook.Button{
		{ID: "confirm", Title: "Confirm"},
		{ID: "cancel", Title: "Cancel"},
	})

	h.post(t, payload([]change{messages(
		receiptTap("wamid.tap-1", "16191234567", "", "Cancel", "wamid.receipt-1"),
	)}))

	stored := h.store.message("wamid.receipt-1")
	assertEqual(t, "the status", h.store.appointment(stored.AppointmentID).status, appointments.Cancelled)
}
