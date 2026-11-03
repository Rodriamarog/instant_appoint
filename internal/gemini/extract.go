package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// recordAppointment is the one thing the model is ever allowed to do.
//
// There is no tool for moving or cancelling an appointment and there must
// never be one. The two failures are not comparable: an appointment this
// writes that nobody agreed to is visible, odd and fixable, while a real
// appointment deleted because a patient said "I might not make it" is gone
// along with the phone number, the reason and any record that it existed. A
// rule the model is told to follow is a rule it can misread; a tool that was
// never offered is one it cannot call.
const recordAppointment = "record_appointment"

// localDateTime is how the model is asked to write a start: a wall clock in
// the business's own zone, with no offset. A conversation says "Friday at
// three", not an instant, and asking the model for an offset invites it to
// invent one.
const localDateTime = "2006-01-02T15:04"

// Appointment is an appointment a conversation agreed on, as the model read
// it.
type Appointment struct {
	// ClientName is the customer's name as they gave it, which may be empty:
	// a thread can settle on a time before anybody says who it is for.
	ClientName string

	// StartsAt is the moment agreed, resolved against the business's zone.
	StartsAt time.Time

	// Certain is whether the conversation named a specific date, a specific
	// time, and assent from both sides. False is the hedged case — "sometime
	// Tuesday", "let me check and get back to you" — which becomes a
	// proposal for a person to look at rather than a booking in the
	// calendar.
	Certain bool

	// Summary is one line saying what was agreed or asked for, for the
	// person who later has to work out why this appointment exists.
	Summary string
}

// ExtractAppointment reads a whole conversation and reports the appointment
// it agreed on, or nil when it agreed none.
//
// history is oldest first and is the conversation entire — unlike
// GenerateReply, nothing is held back as the turn being answered, because
// this is not answering anything.
func (c *Client) ExtractAppointment(ctx context.Context, history []Message) (*Appointment, error) {
	contents := make([]content, 0, len(history))
	for _, message := range history {
		contents = append(contents, content{Role: message.Role, Parts: []part{{Text: message.Content}}})
	}

	// A conversation with no turns cannot have agreed anything, and sending
	// it would spend a call to be told so.
	if len(contents) == 0 {
		return nil, nil
	}

	body, err := c.generate(ctx, request{
		SafetySettings:   []any{},
		GenerationConfig: map[string]any{},
		SystemInstruction: &content{
			Role:  RoleUser,
			Parts: []part{{Text: c.extractionPrompt()}},
		},
		Contents: contents,
		Tools:    []tool{{FunctionDeclarations: []functionDeclaration{recordAppointmentDeclaration}}},
	})
	if err != nil {
		return nil, err
	}

	return c.readAppointment(body)
}

// extractionPrompt is the instruction the reading is done under.
//
// It is a second prompt rather than tools bolted onto the reply, because the
// two calls are made in different circumstances: in observer mode the clinic
// is holding the conversation itself and nothing is being written to the
// customer at all, so there is no reply for a tool to ride along with. The
// reply request therefore still carries no tools, which is what keeps the
// model that talks to a customer structurally unable to book anything.
func (c *Client) extractionPrompt() string {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}

	return "You are reading a WhatsApp conversation between a business and one of its customers.\n" +
		"Today is " + now().In(c.zone).Format(jsDateTime) + ", and both of them are in the " + businessZone + " time zone.\n" +
		"\n" +
		"Your only job is to record an appointment the two of them have discussed, by calling " + recordAppointment + " once.\n" +
		"If the conversation has not discussed a specific appointment at all, call nothing and answer with a single word.\n" +
		"Set certain to true only when the conversation names a day, names a time, and both sides have agreed to it.\n" +
		"Anything vaguer is certain false: a day with no time, a time nobody answered, \"let me check and get back to you\".\n" +
		"If the customer asks to move or cancel an appointment they already have, call " + recordAppointment +
		" with certain false and say so in the summary. You cannot move or cancel anything; a person will read it and decide.\n" +
		"Never invent a date or a time the conversation does not support."
}

// recordAppointmentDeclaration is the function the model may call. The
// descriptions are part of the prompt — they are what the model reads when
// deciding what to put in each field — so they are written for it rather
// than for a developer.
var recordAppointmentDeclaration = functionDeclaration{
	Name: recordAppointment,
	Description: "Record an appointment this conversation has discussed. " +
		"Call it at most once, and only when a specific appointment has been discussed.",
	Parameters: schema{
		Type: "object",
		Properties: map[string]schema{
			"client_name": {
				Type:        "string",
				Description: "The customer's name as they gave it. Empty if they have not given one.",
			},
			"starts_at": {
				Type: "string",
				Description: "When the appointment starts, as YYYY-MM-DDTHH:MM in the business's own time zone. " +
					"If the conversation settled on a day but not a time, give the most likely time and set certain to false.",
			},
			"certain": {
				Type: "boolean",
				Description: "True only if the conversation names a specific date and time that both sides agreed to. " +
					"False for anything partial, hedged, or still being checked, and for a request to move or cancel.",
			},
			"summary": {
				Type:        "string",
				Description: "One short sentence, in the customer's own language, saying what was agreed or what they asked for.",
			},
		},
		Required: []string{"starts_at", "certain", "summary"},
	},
}

// readAppointment pulls the recorded appointment out of a response, or
// reports that the model recorded none.
func (c *Client) readAppointment(body []byte) (*Appointment, error) {
	var parsed response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("gemini: parse the response: %w", err)
	}

	if len(parsed.Candidates) == 0 {
		if parsed.PromptFeedback != nil && parsed.PromptFeedback.BlockReason != "" {
			return nil, fmt.Errorf("gemini: the conversation could not be read (%s)", parsed.PromptFeedback.BlockReason)
		}

		// Not an error: an answer with nothing in it has recorded nothing,
		// and the only consequence is that no appointment is written.
		return nil, nil
	}

	candidate := parsed.Candidates[0]
	if blockedFinishReasons[candidate.FinishReason] {
		return nil, fmt.Errorf("gemini: the conversation could not be read (%s)", candidate.FinishReason)
	}

	for _, p := range candidate.Content.Parts {
		// Only the one function this request declared. A call to anything
		// else is the model inventing a tool, and acting on it would be
		// acting on a name nothing in this package has defined.
		if p.FunctionCall == nil || p.FunctionCall.Name != recordAppointment {
			continue
		}

		var arguments struct {
			ClientName string `json:"client_name"`
			StartsAt   string `json:"starts_at"`
			Certain    bool   `json:"certain"`
			Summary    string `json:"summary"`
		}
		if err := json.Unmarshal(p.FunctionCall.Args, &arguments); err != nil {
			return nil, fmt.Errorf("gemini: read the arguments of %s: %w", recordAppointment, err)
		}

		startsAt, err := time.ParseInLocation(localDateTime, truncateSeconds(arguments.StartsAt), c.zone)
		if err != nil {
			// Returned rather than swallowed. A model that has started
			// writing starts in some other shape stops every appointment
			// from being written, and a silent nil would make that look
			// exactly like a quiet week.
			return nil, fmt.Errorf("gemini: %s gave a start of %q, which is not %s: %w",
				recordAppointment, arguments.StartsAt, localDateTime, err)
		}

		return &Appointment{
			ClientName: arguments.ClientName,
			StartsAt:   startsAt,
			Certain:    arguments.Certain,
			Summary:    arguments.Summary,
		}, nil
	}

	return nil, nil
}

// truncateSeconds drops a seconds field the model added of its own accord.
// Asking for minutes and being given "15:00:00" is the model being tidy, not
// a failure worth refusing an appointment over.
func truncateSeconds(value string) string {
	if len(value) == len("2006-01-02T15:04:05") && value[16] == ':' {
		return value[:16]
	}

	return value
}
