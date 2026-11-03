package pbstore_test

import (
	"context"
	"testing"

	"github.com/pocketbase/dbx"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// anAppointment is one the assistant read out of a conversation, with every
// column it writes filled in.
var anAppointment = webhook.Appointment{
	ClientName:     "Ana Ruiz",
	ClientPhone:    "5216641234567",
	StartsAt:       "2026-10-30T22:00:00.000Z",
	EndsAt:         "2026-10-30T23:00:00.000Z",
	Status:         appointments.Pending,
	CreatedBy:      appointments.Assistant,
	StatusSetBy:    appointments.Assistant,
	StatusSetAt:    "2026-10-28T19:36:00.000Z",
	ConversationID: "conv-1",
	Notes:          "Limpieza dental",
}

// Every column, read back off the row rather than off the struct that wrote
// it. A field spelled wrong here writes nothing and reports no error: the
// appointment simply arrives on the clinic's calendar with no phone number,
// or with no status, which switches off every reminder for it.
func TestWritesAnAppointmentWithItsProvenance(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	if err := store.CreateAppointment(context.Background(), "user000000000001", anAppointment); err != nil {
		t.Fatalf("CreateAppointment: %v", err)
	}

	records, err := app.FindRecordsByFilter(migrations.AppointmentsCollection, "1=1", "", 0, 0)
	if err != nil {
		t.Fatalf("read the appointments back: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("stored %d appointments, want one", len(records))
	}

	for _, field := range []struct {
		name string
		want string
	}{
		{"user_id", "user000000000001"},
		{"client_name", "Ana Ruiz"},
		{"client_phone", "5216641234567"},
		{"start_time", "2026-10-30T22:00:00.000Z"},
		{"end_time", "2026-10-30T23:00:00.000Z"},
		{"status", string(appointments.Pending)},
		{"created_by", string(appointments.Assistant)},
		{"status_set_by", string(appointments.Assistant)},
		{"status_set_at", "2026-10-28T19:36:00.000Z"},
		{"conversation_id", "conv-1"},
		{"notes", "Limpieza dental"},
	} {
		if got := records[0].GetString(field.name); got != field.want {
			t.Errorf("%s = %q, want %q", field.name, got, field.want)
		}
	}
}

// "Why does this exist?" has to be answerable from the appointment itself,
// and this is the half of that the browser reads: a proposal the assistant
// wrote is a row like any other, and the conversation it names is the only
// thing that distinguishes it from one typed at the desk.
func TestAnAppointmentFromAConversationIsFoundByThatConversation(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	if err := store.CreateAppointment(context.Background(), "user000000000001", anAppointment); err != nil {
		t.Fatalf("CreateAppointment: %v", err)
	}

	for _, tc := range []struct {
		name           string
		conversationID string
		want           bool
	}{
		{"the conversation it was written from", "conv-1", true},
		{"a different conversation", "conv-2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.HasAppointmentFromConversation(context.Background(), tc.conversationID)
			if err != nil {
				t.Fatalf("HasAppointmentFromConversation: %v", err)
			}
			if got != tc.want {
				t.Errorf("HasAppointmentFromConversation(%q) = %v, want %v", tc.conversationID, got, tc.want)
			}
		})
	}
}

// Every appointment booked at the desk has an empty conversation_id, which
// is not a conversation and must never answer for one. A lookup that matched
// them would report "already written" for the first clinic conversation ever
// read, and the assistant would never write anything again.
func TestAppointmentsBookedAtTheDeskAnswerForNoConversation(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	atTheDesk := anAppointment
	atTheDesk.ConversationID = ""
	atTheDesk.CreatedBy = appointments.Staff
	if err := store.CreateAppointment(context.Background(), "user000000000001", atTheDesk); err != nil {
		t.Fatalf("CreateAppointment: %v", err)
	}

	already, err := store.HasAppointmentFromConversation(context.Background(), "")
	if err != nil {
		t.Fatalf("HasAppointmentFromConversation: %v", err)
	}
	if already {
		t.Error("an appointment with no conversation answered for the empty conversation id")
	}
}

// The duplicate rule, as the database enforces it rather than as the handler
// checks it.
//
// The check and the write cannot be one operation in Go, and two deliveries
// of one conversation can be in flight at once on two servers. The unique
// index is the only thing that sees the other one — and the right answer to
// losing that race is the appointment that won, not a failed delivery, so
// the second write is a no-op.
func TestASecondAppointmentFromOneConversationIsQuietlyRefused(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	if err := store.CreateAppointment(context.Background(), "user000000000001", anAppointment); err != nil {
		t.Fatalf("the first write: %v", err)
	}

	second := anAppointment
	second.StartsAt = "2026-11-04T18:00:00.000Z"
	if err := store.CreateAppointment(context.Background(), "user000000000001", second); err != nil {
		t.Errorf("the second write from the same conversation failed instead of being ignored: %v", err)
	}

	var count int
	err := app.DB().NewQuery("SELECT COUNT(*) FROM {{appointments}} WHERE [[conversation_id]] = {:id}").
		Bind(dbx.Params{"id": "conv-1"}).Row(&count)
	if err != nil {
		t.Fatalf("count the appointments: %v", err)
	}
	if count != 1 {
		t.Errorf("one conversation produced %d appointments, want one", count)
	}
}

// Two appointments booked at the desk have the same empty conversation_id,
// and a unique index that did not exclude them would stop a clinic booking a
// second appointment at all.
func TestTwoAppointmentsWithNoConversationAreBothKept(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	atTheDesk := anAppointment
	atTheDesk.ConversationID = ""
	if err := store.CreateAppointment(context.Background(), "user000000000001", atTheDesk); err != nil {
		t.Fatalf("the first write: %v", err)
	}
	if err := store.CreateAppointment(context.Background(), "user000000000001", atTheDesk); err != nil {
		t.Fatalf("the second write: %v", err)
	}

	records, err := app.FindRecordsByFilter(migrations.AppointmentsCollection, "1=1", "", 0, 0)
	if err != nil {
		t.Fatalf("read the appointments back: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("stored %d appointments, want both", len(records))
	}
}
