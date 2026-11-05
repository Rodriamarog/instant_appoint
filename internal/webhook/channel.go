package webhook

// Channel is the app a customer is writing from.
//
// The three words are the values of the channel column on conversations and
// whatsapp_messages; migrations pins "whatsapp" as the default for rows that
// predate the column, and internal/pbstore has the test that the spellings
// still agree.
type Channel string

const (
	WhatsApp  Channel = "whatsapp"
	Messenger Channel = "messenger"
	Instagram Channel = "instagram"
)

// ReachedThroughPage reports whether this channel is sent to through a
// Facebook Page — Messenger and Instagram both are — rather than through a
// WhatsApp phone number.
//
// The two families differ in what identifies the customer (a page-scoped id
// rather than a phone number), in how a tap names the appointment it answers
// (a payload this app chose rather than the id of the message tapped), and
// in what can be sent outside the 24-hour window (nothing, for reminders).
func (c Channel) ReachedThroughPage() bool {
	return c == Messenger || c == Instagram
}

// anonymousName is the client name an appointment gets when the conversation
// gave neither a name nor a phone number: client_name is required, and on
// these channels there is no number to fall back on.
func (c Channel) anonymousName() string {
	switch c {
	case Messenger:
		return "Paciente de Messenger"
	case Instagram:
		return "Paciente de Instagram"
	}

	return "Paciente"
}

// The webhook objects Meta names each channel's deliveries with.
const (
	objectPage      = "page"
	objectInstagram = "instagram"
)
