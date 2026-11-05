package webhook

import (
	"strconv"
	"time"
)

// The two change fields this app subscribes to. Meta sends delivery receipts,
// template approval updates and more down the same webhook; everything else is
// skipped.
const (
	// fieldMessages carries what customers send.
	fieldMessages = "messages"

	// fieldEchoes carries copies of what the business sends from the WhatsApp
	// Business app. Without them a conversation the business started, or a
	// human reply typed on a phone, is invisible in the dashboard and invisible
	// to the AI.
	fieldEchoes = "smb_message_echoes"
)

// incomingMessage is one message pulled out of a delivery.
type incomingMessage struct {
	// Channel is the app the message came through.
	Channel Channel

	// AccountKey is the business the delivery was addressed to: the
	// WhatsApp phone_number_id, the Facebook Page id, or the Instagram
	// account id. It is what finds the clinic.
	AccountKey string

	// CustomerPhone is the customer either way: the sender of an inbound
	// message, the recipient of an echo. On WhatsApp it is their number; on
	// Messenger and Instagram it is the page-scoped id Meta names them by,
	// which is the only address those channels have.
	CustomerPhone string

	Content   string
	MessageID string
	Direction Direction

	// Kind says whether this is something the patient typed or a tap on a
	// button under a message the business sent.
	Kind MessageKind

	// TapChoices are the strings the delivery used to name the tapped
	// button, empty for anything that is not a tap. See resolveTap.
	TapChoices []string

	// RepliesTo is Meta's id for the message this one answers, empty when the
	// delivery named none.
	//
	// It is the only thing that can tie a tap to an appointment: the button
	// carries nothing per-appointment — the template holding it is one object
	// shared by every send — so the reminder that was tapped is the
	// identifier, and the row recording that send is what knows which
	// appointment it was about.
	RepliesTo string

	// Timestamp is when Meta says the message was sent, and HasTimestamp says
	// whether it was usable. A missing or nonsensical one is not defaulted to
	// now: the age decides whether a customer gets an automatic reply, and
	// guessing it wrong means either answering a day-old message or silently
	// not answering a live one.
	Timestamp    time.Time
	HasTimestamp bool
}

// extractMessages pulls every message and echo this app can store out of a
// delivery.
//
// The payload is untrusted input decoded into plain maps and slices, so every
// step below asks what a value is before using it, and anything that does not
// answer is skipped rather than rejected. A delivery this app cannot read
// still has to be acknowledged: a non-2xx would have the router redeliver it
// for as long as it keeps failing.
func extractMessages(payload any) []incomingMessage {
	switch object, _ := text(field(payload, "object")); object {
	case objectPage:
		return extractPageMessages(payload, Messenger)
	case objectInstagram:
		return extractPageMessages(payload, Instagram)
	}

	var out []incomingMessage

	for _, entry := range slice(field(payload, "entry")) {
		for _, raw := range slice(field(entry, "changes")) {
			change := asMap(raw)
			value := field(change, "value")

			// Which number the delivery is for. Without it there is no account
			// to attribute the message to.
			phoneNumberID, ok := text(field(field(value, "metadata"), "phone_number_id"))
			if !ok || phoneNumberID == "" {
				continue
			}

			var items []any
			var direction Direction

			switch name, _ := text(field(change, "field")); name {
			case fieldMessages:
				items, direction = slice(field(value, "messages")), Inbound
			case fieldEchoes:
				items, direction = slice(field(value, "message_echoes")), Outbound
			default:
				continue
			}

			for _, item := range items {
				message, ok := readMessage(item, direction, phoneNumberID)
				if !ok {
					continue
				}
				out = append(out, message)
			}
		}
	}

	return out
}

// readMessage reads one message, reporting false for anything this app does
// not store: a kind it cannot render as a conversation turn, one with no id,
// or one with nobody to attribute it to.
func readMessage(item any, direction Direction, phoneNumberID string) (incomingMessage, bool) {
	message := asMap(item)

	extracted, ok := readBody(message)
	if !ok {
		return incomingMessage{}, false
	}

	// The id is the deduplication key. A message without one cannot be
	// recognised on redelivery, so storing it would mean storing it again on
	// every retry.
	id, _ := text(field(message, "id"))
	if id == "" {
		return incomingMessage{}, false
	}

	// An inbound message names the customer as its sender; an echo names them
	// as its recipient. Both end up in the same column.
	phoneField := "from"
	if direction != Inbound {
		phoneField = "to"
	}
	customerPhone, _ := text(field(message, phoneField))
	if customerPhone == "" {
		return incomingMessage{}, false
	}

	extracted.Channel = WhatsApp
	extracted.AccountKey = phoneNumberID
	extracted.CustomerPhone = customerPhone
	extracted.MessageID = id
	extracted.Direction = direction

	// Read for every kind, not only a tap: it costs nothing, and a reply
	// context absent from a shape that was expected to carry one is a fact
	// worth having at the one place that looks for it.
	extracted.RepliesTo, _ = text(field(field(message, "context"), "id"))

	if seconds, ok := unixSeconds(field(message, "timestamp")); ok {
		extracted.Timestamp, extracted.HasTimestamp = time.Unix(seconds, 0).UTC(), true
	}

	return extracted, true
}

// readBody fills in what the message says and how it said it, leaving the
// fields that are the same for every kind to the caller.
//
// Images, audio, documents, reactions and location pins still fall through to
// false here. None of them is something the AI can read or this app can store
// as a conversation turn, and nothing in the app acts on one.
func readBody(message map[string]any) (incomingMessage, bool) {
	switch kind, _ := text(field(message, "type")); kind {
	case "text":
		body, ok := text(field(field(message, "text"), "body"))
		if !ok {
			return incomingMessage{}, false
		}

		return incomingMessage{Kind: KindText, Content: body}, true

	case "button":
		// A tap on a quick reply under a template. Meta reports it as the
		// label the patient saw, in two fields: payload is what the send
		// attached to the button and text is the label itself, and a send
		// that attached nothing — which is every send this app makes — is
		// documented to repeat the label in both.
		button := field(message, "button")
		label, _ := text(field(button, "text"))
		payload, _ := text(field(button, "payload"))

		return readTap(label, payload)

	case "interactive":
		// A tap on an interactive message's buttons, which this app does not
		// send today. It is read anyway because it is the other shape a tap
		// can arrive in and the two are indistinguishable to a patient: if a
		// reminder is ever sent this way, the difference should be that
		// nothing here had to change, not that every tap went quiet.
		interactive := field(message, "interactive")
		if replyKind, _ := text(field(interactive, "type")); replyKind != "button_reply" {
			return incomingMessage{}, false
		}

		reply := field(interactive, "button_reply")
		title, _ := text(field(reply, "title"))
		id, _ := text(field(reply, "id"))

		return readTap(title, id)
	}

	return incomingMessage{}, false
}

// readTap builds a tap out of the label it showed and the identifier the
// delivery gave it, rejecting one that named itself with neither.
//
// The label is what the message reads as in the thread, because that is what
// the patient saw themselves tap. The identifier is kept beside it rather
// than shown: it is a second chance at recognising the button, not a second
// thing to display.
func readTap(label, identifier string) (incomingMessage, bool) {
	if label == "" && identifier == "" {
		return incomingMessage{}, false
	}

	content := label
	if content == "" {
		content = identifier
	}

	return incomingMessage{
		Kind:       KindTap,
		Content:    content,
		TapChoices: []string{label, identifier},
	}, true
}

// extractPageMessages reads a Messenger or Instagram delivery.
//
// Both arrive in the Messenger Platform's shape — entry[].messaging[], each
// event naming a sender, a recipient and a message — and differ only in
// which id entry.id is: the Page's, or the Instagram account's.
func extractPageMessages(payload any, channel Channel) []incomingMessage {
	var out []incomingMessage

	for _, entry := range slice(field(payload, "entry")) {
		business, ok := text(field(entry, "id"))
		if !ok || business == "" {
			continue
		}

		for _, event := range slice(field(entry, "messaging")) {
			message, ok := readPageMessage(event, channel, business)
			if !ok {
				continue
			}
			out = append(out, message)
		}
	}

	return out
}

// readPageMessage reads one messaging event, reporting false for anything
// that is not a message this app stores: a read receipt, a reaction, an
// edit, a deletion, or an attachment with no text.
func readPageMessage(event any, channel Channel, business string) (incomingMessage, bool) {
	message := field(event, "message")
	if message == nil {
		return incomingMessage{}, false
	}

	id, _ := text(field(message, "mid"))
	if id == "" {
		return incomingMessage{}, false
	}

	if deleted, _ := field(message, "is_deleted").(bool); deleted {
		return incomingMessage{}, false
	}

	sender, _ := text(field(field(event, "sender"), "id"))
	recipient, _ := text(field(field(event, "recipient"), "id"))

	// An echo is the business's own message: sent from the Page's inbox,
	// from the Instagram app, or by this app. The customer is then the
	// recipient.
	extracted := incomingMessage{Channel: channel, AccountKey: business, MessageID: id}
	if echo, _ := field(message, "is_echo").(bool); echo {
		extracted.Direction, extracted.CustomerPhone = Outbound, recipient
	} else {
		extracted.Direction, extracted.CustomerPhone = Inbound, sender
	}

	// A customer id equal to the business's own is a delivery about
	// nothing this app can attribute.
	if extracted.CustomerPhone == "" || extracted.CustomerPhone == business {
		return incomingMessage{}, false
	}

	body, hasText := text(field(message, "text"))
	if reply := field(message, "quick_reply"); reply != nil {
		payload, _ := text(field(reply, "payload"))
		tap, ok := readTap(body, payload)
		if !ok {
			return incomingMessage{}, false
		}
		extracted.Kind, extracted.Content, extracted.TapChoices = tap.Kind, tap.Content, tap.TapChoices
	} else {
		if !hasText || body == "" {
			return incomingMessage{}, false
		}
		extracted.Kind, extracted.Content = KindText, body
	}

	extracted.RepliesTo, _ = text(field(field(message, "reply_to"), "mid"))

	// Milliseconds here, where WhatsApp sends seconds.
	if millis, ok := field(event, "timestamp").(float64); ok && millis > 0 {
		extracted.Timestamp, extracted.HasTimestamp = time.UnixMilli(int64(millis)).UTC(), true
	}

	return extracted, true
}

// unixSeconds reads Meta's timestamp, which arrives as a decimal string of
// whole seconds. A zero or negative value is treated as absent rather than as
// 1970: it means the sender did not say when, not that the message is ancient.
func unixSeconds(value any) (int64, bool) {
	switch typed := value.(type) {
	case string:
		seconds, err := strconv.ParseInt(typed, 10, 64)
		if err != nil || seconds <= 0 {
			return 0, false
		}

		return seconds, true

	case float64:
		// Not a shape Meta sends, but a payload is only as well-formed as the
		// sender makes it, and a number here is unambiguous.
		if typed <= 0 || typed != float64(int64(typed)) {
			return 0, false
		}

		return int64(typed), true
	}

	return 0, false
}

// field reads one key of a JSON object, returning nil when the value is not an
// object or has no such key.
func field(value any, name string) any {
	return asMap(value)[name]
}

func asMap(value any) map[string]any {
	object, _ := value.(map[string]any)

	return object
}

// slice returns value as a JSON array, or nil when it is anything else.
func slice(value any) []any {
	array, _ := value.([]any)

	return array
}

// text returns value as a JSON string. The bool distinguishes an absent or
// non-string value from an empty string, which matters for a message body: a
// body of "" is a message this app stores, while a body that is a number is a
// payload it does not understand.
func text(value any) (string, bool) {
	s, ok := value.(string)

	return s, ok
}
