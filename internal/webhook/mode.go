package webhook

// Mode is how much a clinic has allowed the assistant to do with its
// customers' messages. It is the clinic's ai_mode setting, stored on
// business_settings and read in internal/pbstore.
//
// It lives in this package rather than next to the collection it is stored on
// because the only decision it drives is the one below, and that decision has
// to stay answerable with no database — see internal/arch for why.
type Mode string

const (
	// ModeOff is the assistant switched off: it neither replies nor writes.
	// Inbound messages are still stored, so the dashboard shows the whole
	// conversation.
	ModeOff Mode = "off"

	// ModeObserver is the assistant watching a conversation the clinic is
	// handling itself. It does not reply. What it writes for the clinic to
	// read is #27 and is deliberately not this package's business yet.
	ModeObserver Mode = "observer"

	// ModeAutopilot is the assistant answering the customer itself.
	ModeAutopilot Mode = "autopilot"
)

// RepliesAutomatically reports whether a clinic in this mode has its
// customers answered without a human.
//
// It asks "is this autopilot" rather than "is this not off", so that anything
// this build does not recognise — the zero value a clinic with no settings
// row arrives as, a position added by a newer deploy, a value typed into the
// admin UI — leaves the customer unanswered. Silence is recoverable; a bot
// that has already spoken to a patient is not.
func (m Mode) RepliesAutomatically() bool {
	return m == ModeAutopilot
}
