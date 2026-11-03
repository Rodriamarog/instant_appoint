package pbstore_test

import (
	"context"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// saveReminder writes the row internal/reminderjob writes when a reminder
// goes out: the one thing holding both Meta's id for the message and the
// appointment it was about.
func saveReminder(t *testing.T, app core.App, userID, messageID, eventID string) {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("whatsapp_messages")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("to_number", "5215511111111")
	record.Set("message_type", "reminder")
	record.Set("direction", "outbound")
	record.Set("status", "sent")
	record.Set("message_id", messageID)
	record.Set("event_id", eventID)
	record.Set("sent_at", "2026-10-13T11:00:00.000Z")
	if err := app.Save(record); err != nil {
		t.Fatalf("save the reminder row: %v", err)
	}
}

// saveAppointment writes a booking with details on it, so a test can see
// whether a status change left them alone.
func saveAppointment(t *testing.T, app core.App, userID string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("appointments")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("client_name", "Ana López")
	record.Set("client_phone", "5215511111111")
	record.Set("notes", "Trae radiografías")
	record.Set("start_time", "2026-10-14T16:30:00.000Z")
	record.Set("end_time", "2026-10-14T17:00:00.000Z")
	record.Set("status", string(appointments.Pending))
	record.Set("status_set_by", string(appointments.Staff))
	record.Set("status_set_at", "2026-10-01T09:00:00.000Z")
	if err := app.Save(record); err != nil {
		t.Fatalf("save the appointment: %v", err)
	}

	return record.Id
}

func TestAppointmentForSentMessageFindsWhatAReminderWasAbout(t *testing.T) {
	app := newApp(t)
	saveReminder(t, app, "user000000000001", "wamid.reminder", "appointment1")

	got, err := pbstore.NewWebhookStore(app).
		AppointmentForSentMessage(context.Background(), "user000000000001", "wamid.reminder")
	if err != nil {
		t.Fatalf("AppointmentForSentMessage: %v", err)
	}
	if got != "appointment1" {
		t.Errorf("appointment = %q, want appointment1", got)
	}
}

// A tap can answer any message the business ever sent. One answering an
// ordinary reply finds nothing, which is the right answer: acting on it would
// mean moving whatever appointment happened to be nearby.
func TestAppointmentForSentMessageIgnoresEverythingThatIsNotAReminder(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	err := store.SaveMessage(context.Background(), "user000000000001", webhook.StoredMessage{
		MessageID:      "wamid.chat",
		ConversationID: "conv1",
		CustomerPhone:  "5215511111111",
		Content:        "nos vemos el martes",
		Direction:      webhook.Outbound,
		SentAt:         "2026-10-13T11:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	got, err := store.AppointmentForSentMessage(context.Background(), "user000000000001", "wamid.chat")
	if err != nil {
		t.Fatalf("AppointmentForSentMessage: %v", err)
	}
	if got != "" {
		t.Errorf("appointment = %q, want nothing", got)
	}
}

// A wamid matched across clinics would let one clinic's delivery move
// another clinic's appointment.
func TestAppointmentForSentMessageIsScopedToTheClinic(t *testing.T) {
	app := newApp(t)
	saveReminder(t, app, "user000000000002", "wamid.reminder", "appointment1")

	got, err := pbstore.NewWebhookStore(app).
		AppointmentForSentMessage(context.Background(), "user000000000001", "wamid.reminder")
	if err != nil {
		t.Fatalf("AppointmentForSentMessage: %v", err)
	}
	if got != "" {
		t.Errorf("appointment = %q, want nothing", got)
	}
}

func TestAppointmentForSentMessageIsNothingRatherThanAnErrorForAnUnknownID(t *testing.T) {
	app := newApp(t)

	got, err := pbstore.NewWebhookStore(app).
		AppointmentForSentMessage(context.Background(), "user000000000001", "wamid.never-sent")
	if err != nil {
		t.Fatalf("AppointmentForSentMessage: %v", err)
	}
	if got != "" {
		t.Errorf("appointment = %q, want nothing", got)
	}
}

// The three status columns and nothing else. The rest of an appointment is
// somebody's work — a phone number corrected at the desk while the patient
// was tapping — and a status change is not an edit of it.
func TestSetAppointmentStatusWritesTheStatusAndItsProvenanceAndNothingElse(t *testing.T) {
	app := newApp(t)
	id := saveAppointment(t, app, "user000000000001")

	err := pbstore.NewWebhookStore(app).SetAppointmentStatus(
		context.Background(), id, appointments.Cancelled, appointments.Patient, "2026-10-13T12:00:00.000Z",
	)
	if err != nil {
		t.Fatalf("SetAppointmentStatus: %v", err)
	}

	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("read the appointment back: %v", err)
	}

	for _, tc := range []struct{ column, want string }{
		{"status", "cancelled"},
		{"status_set_by", "patient"},
		{"status_set_at", "2026-10-13T12:00:00.000Z"},
		{"client_name", "Ana López"},
		{"client_phone", "5215511111111"},
		{"notes", "Trae radiografías"},
		{"start_time", "2026-10-14T16:30:00.000Z"},
	} {
		if got := record.GetString(tc.column); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.column, got, tc.want)
		}
	}
}

// A tap is stored as its own kind of row, carrying the appointment it moved.
// The thread reads both: the kind to know a button was pressed, the empty
// appointment to say that the press changed nothing.
func TestSaveMessageRecordsATapAsSuchWithTheAppointmentItMoved(t *testing.T) {
	app := newApp(t)

	err := pbstore.NewWebhookStore(app).SaveMessage(context.Background(), "user000000000001", webhook.StoredMessage{
		MessageID:      "wamid.tap",
		Kind:           webhook.KindTap,
		AppointmentID:  "appointment1",
		ConversationID: "conv1",
		CustomerPhone:  "5215511111111",
		Content:        "Confirmar",
		Direction:      webhook.Inbound,
		SentAt:         "2026-10-13T12:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	record, err := app.FindFirstRecordByFilter(
		"whatsapp_messages", "message_id = {:id}", dbx.Params{"id": "wamid.tap"},
	)
	if err != nil {
		t.Fatalf("read the stored tap back: %v", err)
	}

	for _, tc := range []struct{ column, want string }{
		{"message_type", "button"},
		{"event_id", "appointment1"},
		{"message_content", "Confirmar"},
		{"direction", "inbound"},
	} {
		if got := record.GetString(tc.column); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.column, got, tc.want)
		}
	}
}

// A tap this app could not match to anything is still a tap: it is stored,
// and the appointment it carries is empty rather than guessed at. That
// emptiness is the whole surfacing mechanism, so it is pinned here.
func TestSaveMessageStoresAnUnmatchedTapWithNoAppointmentAgainstIt(t *testing.T) {
	app := newApp(t)

	err := pbstore.NewWebhookStore(app).SaveMessage(context.Background(), "user000000000001", webhook.StoredMessage{
		MessageID:      "wamid.tap",
		Kind:           webhook.KindTap,
		ConversationID: "conv1",
		CustomerPhone:  "5215511111111",
		Content:        "Cancelar",
		Direction:      webhook.Inbound,
		SentAt:         "2026-10-13T12:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	record, err := app.FindFirstRecordByFilter(
		"whatsapp_messages", "message_id = {:id}", dbx.Params{"id": "wamid.tap"},
	)
	if err != nil {
		t.Fatalf("read the stored tap back: %v", err)
	}
	if got := record.GetString("event_id"); got != "" {
		t.Errorf("event_id = %q, want nothing", got)
	}
	if got := record.GetString("message_type"); got != "button" {
		t.Errorf("message_type = %q, want button", got)
	}
}
