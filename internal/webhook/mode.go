package webhook

// Mode is how much a clinic has allowed the assistant to do with its
// customers' messages. It is the clinic's ai_mode setting, stored on
// business_settings and read in internal/pbstore.
//
// It lives in this package rather than next to the collection it is stored on
// because the only decisions it drives are the two below, and they have to
// stay answerable with no database — see internal/arch for why.
type Mode string

const (
	// ModeOff is the assistant switched off: it neither replies nor writes,
	// and it does not even read. Inbound messages are still stored, so the
	// dashboard shows the whole conversation.
	ModeOff Mode = "off"

	// ModeObserver is the assistant watching a conversation the clinic is
	// holding itself, from the WhatsApp Business app on somebody's phone. It
	// never replies, but it does read: an agreement reached in that
	// conversation becomes an appointment without anyone retyping it.
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

// RecordsAppointments reports whether a clinic in this mode has its
// conversations read for appointments to write.
//
// This is the line between off and observer, which were behaviourally
// identical until the write path existed: both are silent, and only one of
// them is watching. Autopilot does the same reading — the extraction is the
// same in both, and the mode decides only whether the assistant is also
// talking.
//
// An allow-list for the same reason as above, in the other direction: a
// position this build has never heard of writes nothing into a clinic's
// calendar.
func (m Mode) RecordsAppointments() bool {
	return m == ModeObserver || m == ModeAutopilot
}
