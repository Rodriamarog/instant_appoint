package webhook

import "github.com/Rodriamarog/instant_appoint/internal/appointments"

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
// That is why this is a table of every language rather than a comparison
// against two Spanish words. Each appointment carries the variant its
// reminder is written in, so one clinic sends Spanish to one patient and
// English to the next, and both sets of taps arrive here. Matching only the
// Spanish pair would read half of them as unrecognised and silently leave
// every English patient's appointment untouched.
//
// It mirrors QUICK_REPLIES in web/src/lib/templates.ts, which is what the
// create form offers. A label added there and not here does not fail
// anything: it produces a tap nobody can act on — recorded and surfaced by
// the handler, which is the whole reason that path exists.
var quickReplies = map[string]TapAction{
	"Confirmar": TapConfirm,
	"Cancelar":  TapCancel,
	"Confirm":   TapConfirm,
	"Cancel":    TapCancel,

	// The two ids this app sets on a creation receipt's buttons, which are
	// not labels and are never shown. A receipt is a free-form message
	// rather than a template, so unlike a reminder it can carry a payload
	// this app chose — and a payload is the one part of a tap that does not
	// change with the language the patient reads. The titles above are
	// still matched, because the id is what Meta returns and not what it
	// promises to return.
	confirmButtonID: TapConfirm,
	cancelButtonID:  TapCancel,
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
		if action, ok := quickReplies[choice]; ok {
			return action, true
		}
	}

	return "", false
}
