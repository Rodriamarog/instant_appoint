package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// Everything below writes its appointments before the status column exists
// and only then lets the migration run. That is the whole risk of this one:
// a clinic's calendar is already full when it ships, and an appointment that
// comes out of it with no status is an appointment whose reminder never goes
// out again — silently, because nothing errors.
const theStatusMigration = "1793232007_appointment_status.go"

func TestAnAppointmentBookedBeforeTheStatusColumnExistedIsPending(t *testing.T) {
	app := newAppBefore(t, theStatusMigration)

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id":      "carried-over-owner",
		"client_name":  "Ana Lopez",
		"client_phone": "5216641234567",
		"start_time":   "2026-11-02T17:00:00.000Z",
		"end_time":     "2026-11-02T18:00:00.000Z",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("the appointment booked before the status column is gone: %v", err)
	}

	// Pending and not confirmed: nobody asked this patient anything, and the
	// screen must not claim they said yes.
	if got := record.GetString("status"); got != string(appointments.Pending) {
		t.Errorf("status is %q, want %q — an existing booking that is left blank is one nothing will ever remind about",
			got, appointments.Pending)
	}

	// Staff, because that is the only way an appointment could have been
	// booked before this ticket: there was no assistant and no patient reply
	// path.
	for field, want := range map[string]string{
		"created_by":    string(appointments.Staff),
		"status_set_by": string(appointments.Staff),
	} {
		if got := record.GetString(field); got != want {
			t.Errorf("%s is %q, want %q", field, got, want)
		}
	}

	// "Who said so and when" is the question the screen exists to answer, so
	// a backfilled row has to carry a real moment rather than an empty one.
	// Its creation is when it became pending.
	want := isotime.Format(record.GetDateTime("created").Time())
	if got := record.GetString("status_set_at"); got != want {
		t.Errorf("status_set_at is %q, want the row's creation time %q", got, want)
	}
}

func TestAnAppointmentWrittenWithNoStatusIsPending(t *testing.T) {
	// The column has a create-time default, so a caller that has not heard
	// of status — an older build of the SPA, a script, a seed — books a
	// pending appointment rather than an invisible one.
	app := newAppBefore(t, theStatusMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id":     "owner",
		"client_name": "Beto Ruiz",
		"start_time":  "2026-11-02T17:00:00.000Z",
		"end_time":    "2026-11-02T18:00:00.000Z",
	})

	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}
	if got := record.GetString("status"); got != string(appointments.Pending) {
		t.Errorf("status is %q, want %q", got, appointments.Pending)
	}
}

func TestAProposedAppointmentCanRecordTheConversationItCameFrom(t *testing.T) {
	// The assistant writing an appointment is answering a specific thread,
	// and a receptionist asking "where did this come from" has to be able to
	// get back to it. The link is the id of the conversation row.
	app := newAppBefore(t, theStatusMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id":         "owner",
		"client_name":     "Carla Diaz",
		"start_time":      "2026-11-02T17:00:00.000Z",
		"end_time":        "2026-11-02T18:00:00.000Z",
		"status":          string(appointments.Proposed),
		"created_by":      string(appointments.Assistant),
		"status_set_by":   string(appointments.Assistant),
		"status_set_at":   "2026-11-01T09:00:00.000Z",
		"conversation_id": "conv123",
	})

	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}

	for field, want := range map[string]string{
		"status":          string(appointments.Proposed),
		"created_by":      string(appointments.Assistant),
		"status_set_by":   string(appointments.Assistant),
		"status_set_at":   "2026-11-01T09:00:00.000Z",
		"conversation_id": "conv123",
	} {
		if got := record.GetString(field); got != want {
			t.Errorf("%s is %q, want %q", field, got, want)
		}
	}
}

func TestRollingTheStatusMigrationBackKeepsTheAppointments(t *testing.T) {
	app := newAppBefore(t, theStatusMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id":     "owner",
		"client_name": "Ana Lopez",
		"start_time":  "2026-11-02T17:00:00.000Z",
		"end_time":    "2026-11-02T18:00:00.000Z",
	})

	full := core.MigrationsList{}
	full.Copy(core.SystemMigrations)
	full.Copy(core.AppMigrations)
	if _, err := core.NewMigrationsRunner(app, full).Down(migrationsFrom(theStatusMigration)); err != nil {
		t.Fatalf("revert the status migration: %v", err)
	}

	// The down is what a deploy comes off with. Losing the provenance is the
	// point of reverting it; losing the clinic's calendar with it is not.
	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("the appointment did not survive the rollback: %v", err)
	}
	if got := record.GetString("client_name"); got != "Ana Lopez" {
		t.Errorf("client_name is %q, want %q", got, "Ana Lopez")
	}

	collection, err := app.FindCollectionByNameOrId("appointments")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}
	for _, field := range []string{"status", "status_set_by", "status_set_at", "created_by", "conversation_id"} {
		if collection.Fields.GetByName(field) != nil {
			t.Errorf("%s is still on the collection after the rollback", field)
		}
	}
}
