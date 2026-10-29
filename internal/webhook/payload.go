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

// incomingText is one text message pulled out of a delivery.
type incomingText struct {
	PhoneNumberID string

	// CustomerPhone is the customer either way: the sender of an inbound
	// message, the recipient of an echo.
	CustomerPhone string

	Content   string
	MessageID string
	Direction Direction

	// Timestamp is when Meta says the message was sent, and HasTimestamp says
	// whether it was usable. A missing or nonsensical one is not defaulted to
	// now: the age decides whether a customer gets an automatic reply, and
	// guessing it wrong means either answering a day-old message or silently
	// not answering a live one.
	Timestamp    time.Time
	HasTimestamp bool
}

// extractTexts pulls every text message and echo out of a delivery.
//
// The payload is untrusted input decoded into plain maps and slices, so every
// step below asks what a value is before using it, and anything that does not
// answer is skipped rather than rejected. A delivery this app cannot read
// still has to be acknowledged: a non-2xx would have the router redeliver it
// for as long as it keeps failing.
func extractTexts(payload any) []incomingText {
	var out []incomingText

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
				message, ok := readText(item, direction, phoneNumberID)
				if !ok {
					continue
				}
				out = append(out, message)
			}
		}
	}

	return out
}

// readText reads one message, reporting false for anything this app does not
// store: a non-text message, one with no id, or one with nobody to attribute
// it to.
func readText(item any, direction Direction, phoneNumberID string) (incomingText, bool) {
	message := asMap(item)

	// Images, audio, documents, reactions, location pins and interactive
	// replies all arrive here. None of them is something the AI can read or
	// this app can store as a conversation turn.
	if kind, _ := text(field(message, "type")); kind != "text" {
		return incomingText{}, false
	}

	body, ok := text(field(field(message, "text"), "body"))
	if !ok {
		return incomingText{}, false
	}

	// The id is the deduplication key. A message without one cannot be
	// recognised on redelivery, so storing it would mean storing it again on
	// every retry.
	id, _ := text(field(message, "id"))
	if id == "" {
		return incomingText{}, false
	}

	// An inbound message names the customer as its sender; an echo names them
	// as its recipient. Both end up in the same column.
	phoneField := "from"
	if direction != Inbound {
		phoneField = "to"
	}
	customerPhone, _ := text(field(message, phoneField))
	if customerPhone == "" {
		return incomingText{}, false
	}

	extracted := incomingText{
		PhoneNumberID: phoneNumberID,
		CustomerPhone: customerPhone,
		Content:       body,
		MessageID:     id,
		Direction:     direction,
	}
	if seconds, ok := unixSeconds(field(message, "timestamp")); ok {
		extracted.Timestamp, extracted.HasTimestamp = time.Unix(seconds, 0).UTC(), true
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
