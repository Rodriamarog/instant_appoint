package webhook_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// Messenger and Instagram reach this handler down the same webhook as
// WhatsApp, in the Messenger Platform's shape: entry[].messaging[], with the
// customer named by a page-scoped id rather than a phone number.

const (
	pageID      = "PAGE-1"
	instagramID = "IG-1"
	customerID  = "PSID-42"
)

// connectPages gives the default clinic a Page and the Instagram account
// linked to it. Instagram sends through the Page, which is what SendFrom
// says.
func connectPages(h *harness) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	base := h.store.accounts[phoneNumberID]

	messenger := base
	messenger.AccessToken = "page-token"
	messenger.SendFrom = pageID
	h.store.accounts[pageID] = messenger

	instagram := messenger
	h.store.accounts[instagramID] = instagram
}

func pageDelivery(object, business string, events ...map[string]any) map[string]any {
	items := make([]any, 0, len(events))
	for _, event := range events {
		items = append(items, event)
	}

	return map[string]any{
		"object": object,
		"entry": []any{map[string]any{
			"id":        business,
			"time":      float64(frozen.UnixMilli()),
			"messaging": items,
		}},
	}
}

func pageText(mid, from, to, body string, sentAt time.Time) map[string]any {
	return map[string]any{
		"sender":    map[string]any{"id": from},
		"recipient": map[string]any{"id": to},
		"timestamp": float64(sentAt.UnixMilli()),
		"message":   map[string]any{"mid": mid, "text": body},
	}
}

func pageEcho(mid, from, to, body string, sentAt time.Time) map[string]any {
	event := pageText(mid, from, to, body, sentAt)
	event["message"].(map[string]any)["is_echo"] = true

	return event
}

func pageQuickReply(mid, from, to, title, payload string, sentAt time.Time) map[string]any {
	event := pageText(mid, from, to, title, sentAt)
	event["message"].(map[string]any)["quick_reply"] = map[string]any{"payload": payload}

	return event
}

func TestAMessengerMessageIsStoredAndAnsweredFromThePage(t *testing.T) {
	h := setup(options{})
	connectPages(h)

	assertStatus(t, h.post(t, pageDelivery("page", pageID, pageText("m_1", customerID, pageID, "hola", frozen))), 200)

	stored := h.store.message("m_1")
	assertEqual(t, "channel", stored.Channel, webhook.Messenger)
	assertEqual(t, "customer", stored.CustomerPhone, customerID)
	assertEqual(t, "direction", stored.Direction, webhook.Inbound)

	assertEqual(t, "sent", h.sends(), []send{{
		channel: webhook.Messenger, phoneNumberID: pageID, to: customerID, text: "reply to: hola",
	}})
}

func TestAnInstagramMessageIsAnsweredThroughTheLinkedPage(t *testing.T) {
	h := setup(options{})
	connectPages(h)

	assertStatus(t, h.post(t, pageDelivery("instagram", instagramID, pageText("ig_1", customerID, instagramID, "hola", frozen))), 200)

	assertEqual(t, "channel", h.store.message("ig_1").Channel, webhook.Instagram)
	assertEqual(t, "sent", h.sends(), []send{{
		channel: webhook.Instagram, phoneNumberID: pageID, to: customerID, text: "reply to: hola",
	}})
}

func TestAPageEchoIsTheBusinessesTurnAndIsNotAnswered(t *testing.T) {
	h := setup(options{})
	connectPages(h)

	h.post(t, pageDelivery("page", pageID, pageEcho("m_echo", pageID, customerID, "Te esperamos", frozen)))

	stored := h.store.message("m_echo")
	assertEqual(t, "direction", stored.Direction, webhook.Outbound)
	assertEqual(t, "customer", stored.CustomerPhone, customerID)
	assertEqual(t, "sent", len(h.sends()), 0)
}

func TestAPageDeliveryForAnUnknownPageIsAcknowledgedAndDropped(t *testing.T) {
	h := setup(options{})

	assertStatus(t, h.post(t, pageDelivery("page", "SOMEONE-ELSES", pageText("m_x", customerID, "SOMEONE-ELSES", "hola", frozen))), 200)
	assertEqual(t, "stored", len(h.store.ids()), 0)
}

func TestReadReceiptsAndAttachmentsAreSkipped(t *testing.T) {
	h := setup(options{})
	connectPages(h)

	read := map[string]any{
		"sender": map[string]any{"id": customerID}, "recipient": map[string]any{"id": pageID},
		"timestamp": float64(frozen.UnixMilli()), "read": map[string]any{"mid": "m_0"},
	}
	photo := pageText("m_photo", customerID, pageID, "", frozen)
	delete(photo["message"].(map[string]any), "text")
	photo["message"].(map[string]any)["attachments"] = []any{map[string]any{"type": "image"}}

	assertStatus(t, h.post(t, pageDelivery("page", pageID, read, photo)), 200)
	assertEqual(t, "stored", len(h.store.ids()), 0)
}

// The receipt's buttons carry the appointment in their payload, because a
// Messenger quick reply does not say which message it answers.
func TestAMessengerReceiptGoesToTheConversationAndNamesItsAppointment(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	connectPages(h)

	h.post(t, pageDelivery("page", pageID, pageText("m_1", customerID, pageID, "el jueves a las 4", frozen)))

	receipts := h.receipts()
	if len(receipts) != 1 {
		t.Fatalf("got %d receipts, want 1", len(receipts))
	}
	assertEqual(t, "to", receipts[0].to, customerID)
	assertEqual(t, "from", receipts[0].phoneNumberID, pageID)
	assertEqual(t, "channel", receipts[0].channel, webhook.Messenger)
	assertEqual(t, "payloads", []string{receipts[0].buttons[0].ID, receipts[0].buttons[1].ID},
		[]string{"confirm:appointment-1", "cancel:appointment-1"})

	// No phone was given, so there is no WhatsApp reminder to reach the
	// patient, and the appointment carries none.
	assertEqual(t, "client phone", h.store.written()[0].ClientPhone, "")
}

func TestATapOnAMessengerReceiptMovesTheAppointmentItNames(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	connectPages(h)

	h.post(t, pageDelivery("page", pageID, pageText("m_1", customerID, pageID, "el jueves a las 4", frozen)))
	h.post(t, pageDelivery("page", pageID, pageQuickReply("m_2", customerID, pageID, "Cancelar", "cancel:appointment-1", frozen)))

	assertEqual(t, "status", h.store.appointment("appointment-1").status, appointments.Cancelled)
	assertEqual(t, "by", h.store.appointment("appointment-1").by, appointments.Patient)
	assertEqual(t, "the tap's row", h.store.message("m_2").AppointmentID, "appointment-1")
}

// A payload is whatever the delivery says it is. One naming an appointment
// from another conversation moves nothing.
func TestATapNamingAnotherConversationsAppointmentChangesNothing(t *testing.T) {
	h := setup(options{extract: findsAnAppointment(true)})
	connectPages(h)

	h.post(t, pageDelivery("page", pageID, pageText("m_1", customerID, pageID, "el jueves a las 4", frozen)))
	h.post(t, pageDelivery("page", pageID, pageQuickReply("m_2", "PSID-OTHER", pageID, "Cancelar", "cancel:appointment-1", frozen)))

	assertEqual(t, "status", h.store.appointment("appointment-1").status, appointments.Status(""))
	assertEqual(t, "the tap's row", h.store.message("m_2").AppointmentID, "")
}

// A phone number the patient wrote is how a Messenger booking gets its
// WhatsApp reminder.
func TestAPhoneTheMessengerPatientWroteBecomesTheAppointmentsPhone(t *testing.T) {
	h := setup(options{extract: func(context.Context, []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
		return &webhook.ExtractedAppointment{
			StartsAt:    time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC),
			Certain:     true,
			Summary:     "Consulta",
			ClientPhone: "+52 664 123 4567",
		}, nil
	}})
	connectPages(h)

	h.post(t, pageDelivery("page", pageID, pageText("m_1", customerID, pageID, "mi cel es +52 664 123 4567", frozen)))

	written := h.store.written()[0]
	assertEqual(t, "client phone", written.ClientPhone, "526641234567")
	// No name was given, so the number stands in for one, as on WhatsApp.
	assertEqual(t, "client name", written.ClientName, "526641234567")
}

func TestAnAnonymousMessengerAppointmentIsNamedAfterTheChannel(t *testing.T) {
	h := setup(options{extract: func(context.Context, []webhook.ConversationMessage) (*webhook.ExtractedAppointment, error) {
		return &webhook.ExtractedAppointment{
			StartsAt: time.Date(2026, 10, 30, 22, 0, 0, 0, time.UTC),
			Certain:  true,
			Summary:  "Consulta",
			// Ten digits: no country code, so not guessed at.
			ClientPhone: "6641234567",
		}, nil
	}})
	connectPages(h)

	h.post(t, pageDelivery("instagram", instagramID, pageText("ig_1", customerID, instagramID, "el jueves a las 4", frozen)))

	written := h.store.written()[0]
	assertEqual(t, "client phone", written.ClientPhone, "")
	if !strings.Contains(written.ClientName, "Instagram") {
		t.Errorf("client name = %q, want one naming Instagram", written.ClientName)
	}
}
