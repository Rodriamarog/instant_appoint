package webhook

import (
	"strings"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
)

// TapAction is what a patient asked for by tapping a quick reply under a
// reminder.
//
// Only the two the reminder template offers exist. A tap this app cannot name
// is not a third action: it is a tap it must not act on, which is why the
// resolver below reports whether it recognised one rather than returning a
// default.
type TapAction string

const (
	// TapConfirm is the first button: the patient will be there.
	TapConfirm TapAction = "confirm"

	// TapCancel is the second: the patient will not.
	TapCancel TapAction = "cancel"
)

// Status is the appointment state a tap puts an appointment into.
func (a TapAction) Status() appointments.Status {
	if a == TapCancel {
		return appointments.Cancelled
	}

	return appointments.Confirmed
}

// quickReplyTitles is the pair of button titles one language's patients
// read, in the order the two buttons are always offered in: Confirm first,
// Cancel second.
type quickReplyTitles struct {
	confirm string
	cancel  string
}

// Every title this app can put under a message, by the Meta template
// variant it is written in.
//
// It is the one owner of these four words. A creation receipt titles its
// buttons from here too — see receiptWordings — because a title this app
// shows is a title a tap arrives carrying, and a word written on a button
// in one place and matched in another is a word that can be changed in only
// one of them. The tap that follows would then be unrecognisable, which is
// a patient's cancellation doing nothing at all.
//
// It mirrors QUICK_REPLIES in web/src/lib/templates.ts, which is what the
// create form offers. pt_BR is absent only because it offers the same two
// Spanish words; a variant with words of its own has to be added here.
var quickReplyWords = map[string]quickReplyTitles{
	spanish: {confirm: "Confirmar", cancel: "Cancelar"},
	english: {confirm: "Confirm", cancel: "Cancel"},
}

// quickReplies is every quick-reply label this app can have put under a
// reminder, paired with the position it sits in — index 0 Confirm, index 1
// Cancel.
//
// Position is the whole identity of a quick reply: the two buttons are
// created once per template, in order, and nothing about a template says
// which of them means what. Meta does not report the position back, though —
// a tap arrives carrying the label the patient saw and nothing else — so the
// position has to be recovered from the label, and the only labels that can
// legitimately arrive are the ones this app itself offers when a template is
// created.
//
// That is why this covers every language rather than comparing against two
// Spanish words. Each appointment carries the variant its reminder is
// written in, so one clinic sends Spanish to one patient and English to the
// next, and both sets of taps arrive here. Matching only the Spanish pair
// would read half of them as unrecognised and silently leave every English
// patient's appointment untouched. Building it from quickReplyWords is what
// keeps that true without anybody remembering to: a language this app can
// write a button in is a language it can read a tap back in.
var quickReplies = resolvableTaps()

func resolvableTaps() map[string]TapAction {
	table := map[string]TapAction{
		// The two ids this app sets on a creation receipt's buttons, which
		// are not labels and are never shown. A receipt is a free-form
		// message rather than a template, so unlike a reminder it can carry
		// a payload this app chose — and a payload is the one part of a tap
		// that does not change with the language the patient reads. The
		// titles are still matched below, because the id is what Meta
		// returns and not what it promises to return.
		confirmButtonID: TapConfirm,
		cancelButtonID:  TapCancel,
	}

	for _, words := range quickReplyWords {
		table[words.confirm] = TapConfirm
		table[words.cancel] = TapCancel
	}

	return table
}

// resolveTap names the action a tap asked for, reporting false when it
// recognises none of the ways the tap identified itself.
//
// It is given every string the delivery used to name the button — Meta puts
// the label in more than one field, and which of them is filled depends on
// whether the buttons came from a template or from an interactive message —
// because taking only one of them would turn a shape this app has not seen
// live into silence.
func resolveTap(choices []string) (TapAction, bool) {
	for _, choice := range choices {
		choice, _, _ = strings.Cut(choice, tapSeparator)
		if action, ok := quickReplies[choice]; ok {
			return action, true
		}
	}

	return "", false
}

// tapSeparator joins a button id to the appointment it is about, in the
// payload of a Messenger or Instagram receipt: "confirm:<appointment id>".
const tapSeparator = ":"

// namedTap is the payload of a button that names its appointment.
func namedTap(buttonID, appointmentID string) string {
	return buttonID + tapSeparator + appointmentID
}

// tappedAppointment is the appointment a tap's payload names, or "" when it
// names none — every WhatsApp tap, whose buttons carry no appointment.
//
// Only a payload that starts with one of this app's own button ids counts,
// so a label that happens to contain the separator is not read as one.
func tappedAppointment(choices []string) string {
	for _, choice := range choices {
		action, appointmentID, found := strings.Cut(choice, tapSeparator)
		if !found || appointmentID == "" {
			continue
		}
		if action == confirmButtonID || action == cancelButtonID {
			return appointmentID
		}
	}

	return ""
}
