package webhook_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// findsAnAppointment is a model that reads the same agreement out of any
// conversation it is shown. The conversation that keeps going is the case
// this exists for: the real one behaves this way, because it is handed the
// whole history every time.
func findsAnAppointment(certain bool) func(context.Context, []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
	return func(context.Context, []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
		return &webhook.ExtractedAppointment{
			ClientName: "Ana Ruiz",
			StartsAt:   time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC),
			Certain:    certain,
			Summary:    "Limpieza dental",
		}, nil
	}
}

// Observer is the mode this ticket exists for. Until now it was
// indistinguishable from off: both returned at the reply gate, before the
// model was reached. It has to read the conversation and write what it finds
// while still never saying a word to the customer, because the clinic is
// holding that conversation itself from a phone.
func TestObserverWritesTheAppointmentAndStillSendsNothing(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	response := h.post(t, payload([]change{messages(textMessage("wamid.obs", "5215511111111", "el viernes a las 3", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "sent", h.sends(), []send{})

	written := h.store.written()
	if len(written) != 1 {
		t.Fatalf("wrote %d appointments, want one: %#v", len(written), written)
	}
	if written[0].Status != appointments.Pending {
		t.Errorf("Status = %q, want %q", written[0].Status, appointments.Pending)
	}
	if written[0].userID != "user-1" {
		t.Errorf("the appointment was written for %q, want the clinic that owns the number", written[0].userID)
	}
}

func TestAutopilotRepliesAndWritesTheAppointment(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeAutopilot)

	h.post(t, payload([]change{messages(textMessage("wamid.auto", "5215511111111", "el viernes a las 3", frozen))}))

	if got := len(h.sends()); got != 1 {
		t.Errorf("sent %d replies, want one", got)
	}
	if got := len(h.store.written()); got != 1 {
		t.Errorf("wrote %d appointments, want one", got)
	}
}

// A hedged agreement reaches the clinic as a proposal in both active modes.
// The extraction is identical; the only difference between them is whether
// the assistant is also talking.
func TestAHedgedAgreementIsWrittenAsAProposalInEitherActiveMode(t *testing.T) {
	for _, mode := range []webhook.Mode{webhook.ModeObserver, webhook.ModeAutopilot} {
		h := setup(options{extract: findsAnAppointment(false)})
		h.store.setMode(mode)

		h.post(t, payload([]change{messages(textMessage("wamid."+string(mode), "5215511111111", "igual el martes", frozen))}))

		written := h.store.written()
		if len(written) != 1 {
			t.Fatalf("in %q: wrote %d appointments, want one", mode, len(written))
		}
		if written[0].Status != appointments.Proposed {
			t.Errorf("in %q: Status = %q, want %q", mode, written[0].Status, appointments.Proposed)
		}
	}
}

// Off means off. The message is still stored — the clinic sees its own
// conversations either way — but nothing reads it and nothing is written.
// Anything this build does not recognise behaves the same, because the
// decision is an allow-list.
func TestAnAssistantThatIsNotSwitchedOnReadsNothingAndWritesNothing(t *testing.T) {
	for _, mode := range []webhook.Mode{webhook.ModeOff, "", "something-a-newer-deploy-added"} {
		h := setup(options{extract: findsAnAppointment(true)})
		h.store.setMode(mode)

		h.post(t, payload([]change{messages(textMessage("wamid.quiet", "5215511111111", "el viernes a las 3", frozen))}))

		assertEqual(t, "stored", h.store.ids(), []string{"wamid.quiet"})
		if got := len(h.readings()); got != 0 {
			t.Errorf("in %q the model was asked to read %d conversations, want none", mode, got)
		}
		if got := len(h.store.written()); got != 0 {
			t.Errorf("in %q %d appointments were written, want none", mode, got)
		}
	}
}

// The trap this feature is designed around. The model sees the whole
// conversation on every inbound message, so the agreement reached in the
// first one is still there in the fourth — and a conversation that carries
// on afterwards, as they all do, would leave the clinic with four copies of
// one appointment and no way to tell which is real.
func TestAConversationThatKeepsGoingDoesNotWriteTheAppointmentAgain(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	for _, text := range []string{"el viernes a las 3", "perfecto", "¿llevo algo?", "gracias"} {
		h.post(t, payload([]change{messages(textMessage("wamid."+text, "5215511111111", text, frozen))}))
	}

	if got := len(h.store.written()); got != 1 {
		t.Errorf("one conversation produced %d appointments, want one", got)
	}
}

// And it stops before the model, not after it. The guard is the cheapest
// thing in the path and the model is the most expensive, so a thread that
// runs for fifty messages after an agreement must not pay for fifty
// readings of it.
func TestOnceAnAppointmentExistsTheConversationIsNotReadAgain(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{messages(textMessage("wamid.one", "5215511111111", "el viernes a las 3", frozen))}))
	h.post(t, payload([]change{messages(textMessage("wamid.two", "5215511111111", "gracias", frozen))}))

	if got := len(h.readings()); got != 1 {
		t.Errorf("the model read the conversation %d times, want once", got)
	}
}

// In observer the clinic is answering from its own phone, and its answer is
// usually the half that settles the appointment — "perfecto, la veo el
// viernes a las 3". Those replies arrive as echoes, so an extraction that
// only ran on inbound messages would wait for the patient to say something
// else before it ever saw the agreement.
func TestTheClinicsOwnRepliesAreReadTooWithoutBeingAnswered(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{echoes(echoMessage("wamid.echo", "5215511111111", "perfecto, la veo el viernes a las 3", frozen))}))

	if got := len(h.readings()); got != 1 {
		t.Errorf("the clinic's own reply was read %d times, want once", got)
	}
	assertEqual(t, "sent", h.sends(), []send{})
}

// The model is given the conversation as stored, newest turn included —
// unlike the reply, which holds the triggering message back as the turn
// being answered. Nothing is being answered here, and the message that most
// likely contains the agreement is the newest one.
func TestTheExtractionReadsTheWholeConversationIncludingTheNewestTurn(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(false)})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{messages(textMessage("wamid.first", "5215511111111", "hola", frozen))}))

	readings := h.readings()
	if len(readings) != 1 {
		t.Fatalf("the model read %d conversations, want one", len(readings))
	}
	assertEqual(t, "the conversation read", readings[0], []webhook.ConversationMessage{
		{Role: webhook.RoleUser, Content: "hola"},
	})
}

// Writing is the last thing in the path and the message is already stored,
// which is the half that must not be lost. A failed write is logged and the
// delivery acknowledged: a non-2xx would have the router redeliver, and the
// redelivery would be recognised as a duplicate and store nothing at all.
func TestAFailedAppointmentWriteStillAcknowledgesTheDelivery(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	h.store.setMode(webhook.ModeObserver)
	h.store.failCreateAppointment = errors.New("the appointment could not be written")

	response := h.post(t, payload([]change{messages(textMessage("wamid.fail", "5215511111111", "el viernes a las 3", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "stored", h.store.ids(), []string{"wamid.fail"})
}

// A model that cannot be reached must not cost the clinic the message. The
// same reasoning as the reply: the storage already happened, and nothing
// after it is worth a redelivery.
func TestAFailedReadingStillAcknowledgesTheDelivery(t *testing.T) {
	h := setup(options{
		extract: func(context.Context, []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
			return nil, errors.New("gemini is unreachable")
		},
	})
	h.store.setMode(webhook.ModeObserver)

	response := h.post(t, payload([]change{messages(textMessage("wamid.unread", "5215511111111", "hola", frozen))}))

	assertStatus(t, response, http.StatusOK)
	assertEqual(t, "stored", h.store.ids(), []string{"wamid.unread"})
	if got := len(h.store.written()); got != 0 {
		t.Errorf("%d appointments were written from a reading that failed, want none", got)
	}
}

// Nothing agreed, nothing written. Most conversations are a question about
// the opening hours, and none of them may leave an appointment behind.
func TestAConversationThatAgreedNothingWritesNothing(t *testing.T) {
	h := setup(options{})
	h.store.setMode(webhook.ModeObserver)

	h.post(t, payload([]change{messages(textMessage("wamid.chat", "5215511111111", "¿a qué hora abren?", frozen))}))

	if got := len(h.store.written()); got != 0 {
		t.Errorf("%d appointments were written from a conversation that agreed nothing", got)
	}
}

// The asymmetry, made structural rather than promised.
//
// The step that reads conversations is handed an AppointmentWriter and not
// the whole Store, so what it is able to do to an appointment is decided by
// the compiler. Creating one is on this interface. Moving or deleting one is
// not, and must never be: a phantom appointment is visible, odd and
// recoverable, while a real appointment deleted because a patient wrote "I
// might not make it" is gone with the time, the number and the reason, and
// nobody ever learns it was there.
//
// Store does carry SetAppointmentStatus, and that is not a hole in this. It
// belongs to a patient pressing Confirmar or Cancelar on a reminder this app
// sent — a deliberate answer, matched to one appointment by the id Meta gave
// that reminder, recorded as the patient's doing. The thing that must never
// happen is the model reading a cancellation out of free text and acting on
// it, and the step that reads free text cannot reach that method.
func TestTheExtractionStepIsHandedNoWayToChangeOrRemoveAnAppointment(t *testing.T) {
	writer := reflect.TypeOf((*webhook.AppointmentWriter)(nil)).Elem()

	got := make([]string, 0, writer.NumMethod())
	for i := range writer.NumMethod() {
		got = append(got, writer.Method(i).Name)
	}

	// Named in full rather than filtered for forbidden words: a method this
	// list does not expect is one somebody has to justify here, whatever it
	// is called.
	want := []string{"CreateAppointment", "HasAppointmentFromConversation"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("webhook.AppointmentWriter offers %v, want exactly %v", got, want)
	}
}

// And the whole Store satisfies it, so narrowing the parameter costs the
// wiring nothing.
var _ webhook.AppointmentWriter = (webhook.Store)(nil)
