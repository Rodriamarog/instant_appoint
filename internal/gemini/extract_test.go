package gemini_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/gemini"
)

// aConversation is the history every case below is read out of. Its content
// is beside the point — the stub decides what comes back — but it has to be
// a real conversation, because an empty one is never sent at all.
var aConversation = []gemini.Message{
	{Role: gemini.RoleUser, Content: "¿tienen lugar el viernes?"},
	{Role: gemini.RoleModel, Content: "sí, ¿a qué hora le queda bien?"},
}

// noCall is the answer the model gives when it read the conversation and
// found no appointment in it: an ordinary text part and no function call.
const noCall = `{"candidates":[{"content":{"parts":[{"text":"nothing was agreed"}],"role":"model"},"finishReason":"STOP"}]}`

// aCall is the answer that records one.
const aCall = `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"record_appointment","args":{` +
	`"client_name":"Ana Ruiz","starts_at":"2026-10-30T15:00","certain":true,"summary":"Limpieza dental"}}}]},` +
	`"finishReason":"STOP"}]}`

// The model is handed exactly one thing it may do, and that thing writes a
// new appointment. There is deliberately no tool for moving or cancelling
// one: a phantom appointment is embarrassing and recoverable, and a real
// appointment deleted because somebody said "I might not make it" is neither.
// Leaving the ability off the wire is the only version of that rule the model
// cannot talk its way around.
func TestOffersTheModelNoWayToMoveOrCancelAnAppointment(t *testing.T) {
	client, got := stub(t, http.StatusOK, noCall)

	if _, err := client.ExtractAppointment(context.Background(), aConversation); err != nil {
		t.Fatalf("ExtractAppointment: %v", err)
	}

	declarations := functionDeclarations(t, got)
	if len(declarations) != 1 {
		t.Fatalf("the model was offered %d functions, want exactly one: %#v", len(declarations), declarations)
	}

	name, _ := declarations[0]["name"].(string)
	if name != "record_appointment" {
		t.Errorf("the one function offered is %q, want record_appointment", name)
	}
	for _, forbidden := range []string{"cancel", "delete", "move", "resched", "update", "change"} {
		if strings.Contains(strings.ToLower(name), forbidden) {
			t.Errorf("the model is offered %q, which can %s an appointment", name, forbidden)
		}
	}
}

// functionDeclarations digs the declared tools out of a captured request.
func functionDeclarations(t *testing.T, got *capture) []map[string]any {
	t.Helper()

	tools, _ := got.body["tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("the request carried no tools: %#v", got.body)
	}

	var out []map[string]any
	for _, tool := range tools {
		entry, _ := tool.(map[string]any)
		for _, declaration := range entry["functionDeclarations"].([]any) {
			declared, _ := declaration.(map[string]any)
			out = append(out, declared)
		}
	}

	return out
}

// Nothing agreed means nothing written. A conversation where the customer
// asked the opening hours must not leave an appointment behind, and the model
// saying so in prose rather than by calling the function is how it says that.
func TestReportsNoAppointmentWhenTheModelCallsNothing(t *testing.T) {
	client, _ := stub(t, http.StatusOK, noCall)

	extracted, err := client.ExtractAppointment(context.Background(), aConversation)
	if err != nil {
		t.Fatalf("ExtractAppointment: %v", err)
	}
	if extracted != nil {
		t.Errorf("an appointment was extracted from a conversation the model declined to record: %#v", extracted)
	}
}

// The arguments as the model writes them, read back as the values the write
// path needs. starts_at is a local wall clock with no offset, because that is
// how a conversation says "Friday at three"; resolving it against the
// business's zone here is what stops an appointment landing seven hours out.
func TestReadsTheRecordedAppointment(t *testing.T) {
	client, _ := stub(t, http.StatusOK, aCall)

	extracted, err := client.ExtractAppointment(context.Background(), aConversation)
	if err != nil {
		t.Fatalf("ExtractAppointment: %v", err)
	}
	if extracted == nil {
		t.Fatal("the model called record_appointment and nothing came back")
	}

	if extracted.ClientName != "Ana Ruiz" {
		t.Errorf("ClientName = %q, want Ana Ruiz", extracted.ClientName)
	}
	if !extracted.Certain {
		t.Error("Certain = false, want true: the model said the agreement was definite")
	}
	if extracted.Summary != "Limpieza dental" {
		t.Errorf("Summary = %q, want Limpieza dental", extracted.Summary)
	}

	// 15:00 in Tijuana on 30 October 2026 is 22:00 UTC: daylight saving is
	// still in force, so the offset is seven hours and not eight.
	if want := time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC); !extracted.StartsAt.Equal(want) {
		t.Errorf("StartsAt = %s, want %s", extracted.StartsAt.UTC(), want)
	}
}

// The hedged half of the split. "Sometime Tuesday" is still worth recording,
// and the one thing that decides whether it reaches the calendar or a
// person's attention is this flag.
func TestReadsAnUncertainAppointment(t *testing.T) {
	const hedged = `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"record_appointment","args":{` +
		`"starts_at":"2026-11-03T09:00","certain":false,"summary":"Dijo que el martes le queda, sin hora"}}}]}}]}`

	client, _ := stub(t, http.StatusOK, hedged)

	extracted, err := client.ExtractAppointment(context.Background(), aConversation)
	if err != nil {
		t.Fatalf("ExtractAppointment: %v", err)
	}
	if extracted == nil {
		t.Fatal("the model recorded a hedged appointment and nothing came back")
	}
	if extracted.Certain {
		t.Error("Certain = true for an appointment the model marked uncertain")
	}
	// Absent in the arguments, so it has to read as absent rather than as
	// some leftover from the last call.
	if extracted.ClientName != "" {
		t.Errorf("ClientName = %q, want empty: the conversation never gave one", extracted.ClientName)
	}
}

// A start the model wrote in some shape this cannot read stops the
// appointment being written, and it has to say so. Reporting nothing would
// make a model that has started answering in ISO-with-offset look exactly
// like a quiet week, for as long as it took anyone to notice.
func TestReportsAStartItCannotRead(t *testing.T) {
	const unreadable = `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"record_appointment","args":{` +
		`"starts_at":"next Tuesday","certain":true,"summary":"x"}}}]}}]}`

	client, _ := stub(t, http.StatusOK, unreadable)

	extracted, err := client.ExtractAppointment(context.Background(), aConversation)
	if err == nil {
		t.Fatalf("ExtractAppointment accepted %#v and returned no error", extracted)
	}
	if !strings.Contains(err.Error(), "next Tuesday") {
		t.Errorf("error = %q, want it to quote the start it could not read", err)
	}
}

// The two calls are separate on purpose, and this is the half that keeps
// them separate. The reply is the only thing a customer ever reads, and it
// is written by a request that offers no way to write anything down — so the
// model answering a patient cannot book in the same breath as promising.
func TestTheReplyRequestOffersTheModelNoTools(t *testing.T) {
	client, got := stub(t, http.StatusOK, oneCandidate)

	if _, err := client.GenerateReply(context.Background(), nil, "hola"); err != nil {
		t.Fatalf("GenerateReply: %v", err)
	}

	if tools, present := got.body["tools"]; present {
		t.Errorf("the reply request declared tools: %#v", tools)
	}
}
