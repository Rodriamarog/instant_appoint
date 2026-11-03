package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// Every case below writes its appointments before the language column exists
// and only then lets the migration run, because that is the whole risk of
// this one. A clinic's calendar is already full when it ships, and until now
// every one of those reminders went out in the one language the business
// configured. A backfill that invented a different answer would change the
// language a patient is written to in without anybody deciding to.
const theLanguageMigration = "1793232009_appointment_language.go"

func TestAnAppointmentBookedBeforeTheLanguageColumnKeepsTheLanguageItWasBeingSentIn(t *testing.T) {
	app := newAppBefore(t, theLanguageMigration)

	// Two businesses, configured differently, which is the case a single
	// hard-coded default would get wrong for one of them.
	saveUnder(t, app, "business_settings", map[string]any{
		"user_id": "san-diego", "timing_minutes": 1440, "is_active": true,
		"template_name": "appointment_reminder", "template_language": "en_US",
		"timezone": "America/Tijuana",
	})
	saveUnder(t, app, "business_settings", map[string]any{
		"user_id": "tijuana", "timing_minutes": 1440, "is_active": true,
		"template_name": "appointment_reminder", "template_language": "es",
		"timezone": "America/Tijuana",
	})

	english := saveUnder(t, app, "appointments", map[string]any{
		"user_id": "san-diego", "client_name": "Ann Cole",
		"start_time": "2026-11-02T17:00:00.000Z", "end_time": "2026-11-02T18:00:00.000Z",
	})
	spanish := saveUnder(t, app, "appointments", map[string]any{
		"user_id": "tijuana", "client_name": "Ana Lopez",
		"start_time": "2026-11-02T17:00:00.000Z", "end_time": "2026-11-02T18:00:00.000Z",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	for id, want := range map[string]string{english: "en_US", spanish: "es"} {
		record, err := app.FindRecordById("appointments", id)
		if err != nil {
			t.Fatalf("the appointment booked before the language column is gone: %v", err)
		}
		if got := record.GetString("language"); got != want {
			t.Errorf("language is %q, want %q — the variant this business's reminders were already going out in",
				got, want)
		}
	}
}

func TestAnAppointmentWhoseOwnerHasNoSettingsRowFallsBackToSpanish(t *testing.T) {
	// An account that never opened the reminders dialog has no settings row
	// at all, and a correlated subquery over nothing returns NULL. Left
	// there, the column would be empty — which the reminder job reads as a
	// language it cannot write and holds the reminder back for.
	app := newAppBefore(t, theLanguageMigration)

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id": "never-configured", "client_name": "Ana Lopez",
		"start_time": "2026-11-02T17:00:00.000Z", "end_time": "2026-11-02T18:00:00.000Z",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}
	if got := record.GetString("language"); got != "es" {
		t.Errorf("language is %q, want \"es\"", got)
	}
}

func TestAnAppointmentWrittenWithNoLanguageIsSpanish(t *testing.T) {
	// The column has a create-time default, so a caller that has not heard
	// of it — an older build of the SPA, a script, the webhook — books an
	// appointment somebody is reminded about rather than one the job holds
	// back as unwritable.
	app := newAppBefore(t, theLanguageMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id": "owner", "client_name": "Beto Ruiz",
		"start_time": "2026-11-02T17:00:00.000Z", "end_time": "2026-11-02T18:00:00.000Z",
	})

	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}
	if got := record.GetString("language"); got != "es" {
		t.Errorf("language is %q, want \"es\"", got)
	}
}

func TestRollingTheLanguageMigrationBackKeepsTheAppointments(t *testing.T) {
	app := newAppBefore(t, theLanguageMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id": "owner", "client_name": "Ana Lopez",
		"start_time": "2026-11-02T17:00:00.000Z", "end_time": "2026-11-02T18:00:00.000Z",
		"language": "en_US",
	})

	full := core.MigrationsList{}
	full.Copy(core.SystemMigrations)
	full.Copy(core.AppMigrations)
	if _, err := core.NewMigrationsRunner(app, full).Down(migrationsFrom(theLanguageMigration)); err != nil {
		t.Fatalf("revert the language migration: %v", err)
	}

	// Losing the per-appointment language is the point of reverting it;
	// losing the clinic's calendar with it is not.
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
	if collection.Fields.GetByName("language") != nil {
		t.Error("language is still on the collection after the rollback")
	}
}
