package webhook

// Outreach is whether a clinic lets the assistant message a patient of its
// own accord. It is the clinic's patient_outreach setting, stored on
// business_settings and read in internal/pbstore.
//
// It is a separate question from Mode, not a fourth position on it. Mode
// says what the assistant may do with a conversation a patient started;
// this says whether the assistant may start one. They were tangled
// together once and the result was a clinic in Observador — told in
// Configuración that the assistant does not reply — having its patients
// messaged by the creation receipt anyway.
//
// It lives beside Mode, and in this package rather than next to the column
// it is stored on, because the one decision it drives has to stay
// answerable with no database. See internal/arch for why.
type Outreach string

const (
	// OutreachOn is the assistant allowed to send a patient a message
	// nobody asked it for. Today that is exactly one message: the receipt
	// that goes out when the assistant writes an appointment.
	OutreachOn Outreach = "on"

	// OutreachOff is the clinic asking for silence except when a patient
	// has written in. It does not stop the clinic's own reminders, which
	// are scheduled messages it configured itself, and it does not stop
	// autopilot answering somebody who just messaged the clinic.
	OutreachOff Outreach = "off"
)

// MessagesPatients reports whether this clinic lets the assistant write to a
// patient who has not just written to it.
//
// Only a deliberate off silences it. That is the opposite reading from
// Mode.RepliesAutomatically next door, and deliberately so, because the two
// failures are not the same shape. An unrecognised mode read as permission
// means a patient is answered by a bot their clinic never switched on, and
// nothing can unsay it. An unrecognised value here read as silence means a
// clinic whose calendar is still being written to by the assistant loses
// the one message that puts those appointments in front of the person who
// knows whether they agreed to them — the write path left running with
// nothing watching it.
//
// So this fails towards the check being sent, and the default stored in
// migrations/1793232012_patient_outreach.go says the same thing.
func (o Outreach) MessagesPatients() bool {
	return o != OutreachOff
}
