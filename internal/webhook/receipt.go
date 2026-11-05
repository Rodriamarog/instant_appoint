package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// Button is one quick reply under a message this app sends.
//
// ID is a payload this app chooses, which Meta hands back untouched when the
// button is tapped. It exists because the title does not survive
// translation: the two buttons are told apart by position, the position has
// to be recovered from what the tap carries, and a title is whatever
// language the patient reads. See quickReplies, which resolves both.
type Button struct {
	ID    string
	Title string
}

// The ids this app sets on a receipt's buttons. They are not labels and are
// never shown: they are the one part of a tap that says the same thing in
// every language, which is what a template's quick replies could not carry
// — a template is one object shared by every send, so nothing per-send can
// be attached to its buttons at all.
const (
	confirmButtonID = "confirm"
	cancelButtonID  = "cancel"
)

// receiptWording is one language's worth of the receipt.
//
// The dates inside it come from internal/reminders rather than from a table
// here, so the receipt and the reminder that follows it name the same day in
// the same words. The button titles come from quickReplyWords in tap.go for
// a sharper version of the same reason: the table that reads a tap back has
// to be the table that titled the button, or a title changed here is a
// patient's answer this app can no longer place. What is left here is only
// the sentence around the dates.
type receiptWording struct {
	// body takes the date and then the time, in that order.
	body string

	buttons quickReplyTitles
}

// How a receipt is written, per Meta template variant.
//
// Keyed by the same variant codes the appointment's language column holds
// and internal/reminders spells dates for, because the sentence and the date
// inside it have to be one language. A variant missing here is a receipt
// that does not go out — the appointment is already written and the reminder
// will still arrive, which is a far better outcome than a Spanish sentence
// wrapped around an English date.
var receiptWordings = map[string]receiptWording{
	spanish: {
		body:    "Tu cita quedó registrada para el %s a las %s. ¿Es correcto?",
		buttons: quickReplyWords[spanish],
	},
	english: {
		body:    "Your appointment is booked for %s at %s. Is that right?",
		buttons: quickReplyWords[english],
	},
}

// sendReceipt tells the patient what the assistant just wrote down and asks
// them to confirm it.
//
// Every failure here is returned rather than acted on, and the only caller
// logs it: the appointment is the valuable thing and it is already stored,
// so a receipt that cannot be composed or cannot be sent must cost nothing
// but itself.
func (h *handler) sendReceipt(
	ctx context.Context,
	account Account,
	message incomingMessage,
	conversationID, to string,
	created createdAppointment,
) error {
	appointment := created.Appointment

	start, err := isotime.Parse(appointment.StartsAt)
	if err != nil {
		return fmt.Errorf("read back the appointment's start time: %w", err)
	}

	location, err := time.LoadLocation(account.Timezone)
	if err != nil {
		return fmt.Errorf("load the clinic's time zone %q: %w", account.Timezone, err)
	}

	wording, ok := receiptWordings[appointment.Language]
	if !ok {
		return fmt.Errorf("no receipt is written in %q", appointment.Language)
	}

	date, clock, ok := reminders.SpellMoment(start, location, appointment.Language)
	if !ok {
		return fmt.Errorf("no date can be spelled in %q", appointment.Language)
	}

	body := fmt.Sprintf(wording.body, date, clock)
	buttons := []Button{
		// Confirm first and cancel second, which is the order every quick
		// reply this app offers is in. The order is not decoration: it is
		// what the two buttons mean, and a tap is matched back to it.
		{ID: confirmButtonID, Title: wording.buttons.confirm},
		{ID: cancelButtonID, Title: wording.buttons.cancel},
	}

	// On Messenger and Instagram a tap does not say which message it
	// answers, so the buttons say which appointment they are about.
	if message.Channel.ReachedThroughPage() {
		for i := range buttons {
			buttons[i].ID = namedTap(buttons[i].ID, created.ID)
		}
	}

	messageID, err := h.deps.SendButtons(ctx, message.Channel, sendFrom(account, message), account.AccessToken, to, body, buttons)
	if err != nil {
		return fmt.Errorf("send the receipt: %w", err)
	}

	// The row that makes the receipt's buttons work. A tap is matched to an
	// appointment through the one row holding both Meta's id for the
	// message tapped and the appointment it was about — the same join a
	// reminder's taps go through. Without this write the patient's Confirm
	// arrives naming a message this app has no record of sending, and
	// nothing moves.
	err = h.deps.Store.SaveMessage(ctx, account.UserID, StoredMessage{
		MessageID:      messageID,
		Kind:           KindReceipt,
		AppointmentID:  created.ID,
		ConversationID: conversationID,
		Channel:        message.Channel,
		CustomerPhone:  to,
		Content:        body,
		Direction:      Outbound,
		SentAt:         isotime.Format(h.deps.Now()),
	})
	if err != nil {
		return fmt.Errorf("store the receipt: %w", err)
	}

	h.deps.Logger.Info("sent a creation receipt",
		"appointment_id", created.ID, "message_id", messageID, "to", to)

	return nil
}

// suppressionReason says why this appointment must not be checked with the
// patient, and is empty when nothing stops it.
//
// The reason is returned rather than logged here so that the caller logs one
// line with the appointment on it, and so that this stays a question with no
// side effects.
func (h *handler) suppressionReason(account Account, channel Channel, to string, appointment Appointment) string {
	// Nothing can reach this today: the one caller is handed the
	// appointment recordAppointment has just written, and that one is
	// always the assistant's. It is here because of what the message says.
	// A receipt tells the patient what the assistant wrote down out of the
	// conversation they were just having; sent about a booking a member of
	// staff made at the desk or over the phone, it asks them to confirm
	// something no conversation of theirs led to. A second caller is all
	// that stands between here and that, and it would arrive silently —
	// this line turns it into a refusal somebody can read in the log.
	if appointment.CreatedBy != appointments.Assistant {
		return "the assistant did not write this appointment"
	}

	// The clinic's own instruction, and the only reason here it gave
	// deliberately. It is checked before the rules this app reasoned out
	// for itself because none of them are an argument against it: a clinic
	// that has said the assistant may not message its patients unprompted
	// has said so about every appointment, however the assistant came to
	// write it.
	if !account.Outreach.MessagesPatients() {
		return "this clinic has turned off messages the assistant sends by itself"
	}

	if appointment.Status == appointments.Proposed {
		// Nothing has been agreed. A proposal is a note for a member of
		// staff to read, and asking the patient to confirm something the
		// clinic has not offered them is how a phantom appointment becomes
		// a real one.
		return "nothing has been agreed yet"
	}

	if to == "" {
		return "there is nobody to send it to"
	}

	start, err := isotime.Parse(appointment.StartsAt)
	if err != nil {
		return "the appointment's start time cannot be read"
	}

	// The lead-time rule. An appointment booked inside its own lead time
	// has its reminder fall due the moment it exists, so the patient would
	// get a receipt and then a near-identical reminder seconds later, both
	// asking them to confirm the same thing. When the reminder is already
	// due, the reminder is the receipt.
	//
	// Only when a reminder will actually go out, though: reminders switched
	// off, or no template chosen, means there is no second message to defer
	// to and suppressing this one would leave the patient with nothing.
	//
	// And only when that reminder can reach the patient: it goes out over
	// WhatsApp, to the appointment's number, which a Messenger or Instagram
	// patient may never have given.
	remindedAnyway := account.SendsReminders && appointment.ClientPhone != ""
	if remindedAnyway && !h.deps.Now().Before(start.Add(-account.ReminderLeadTime)) {
		return "the appointment's own reminder is already due and says the same thing"
	}

	return ""
}
