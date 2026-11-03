package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/migrations"
)

// theAIModeMigration is the file that adds the switch. Everything below
// starts from the database as it was before it, because the claim under test
// is about rows the migration did not write.
const theAIModeMigration = "1793232008_ai_mode.go"

// The assistant answered every clinic's customers, permanently, with no way
// to stop it. Turning it off for new clinics is a field default; turning it
// off for the clinics that are already running is this, and only this, and
// nothing in a fresh database can tell the two apart.
func TestEveryClinicThatExistedBeforeThisTicketStartsOff(t *testing.T) {
	app := newAppBefore(t, theAIModeMigration)

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":           "clinic-already-running",
		"timing_minutes":    120,
		"is_active":         true,
		"template_name":     "appointment_reminder",
		"template_language": "es",
		"timezone":          "America/Hermosillo",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	record, err := app.FindRecordById(migrations.BusinessSettingsCollection, id)
	if err != nil {
		t.Fatalf("the settings row written before the switch is gone: %v", err)
	}

	// Spelled out rather than merely "not autopilot": the settings dialog
	// matches the stored value against the three positions, and a row left
	// at "" would show a clinic no position selected at all.
	if got := record.GetString(migrations.AIModeField); got != migrations.DefaultAIMode {
		t.Errorf("ai_mode is %q, want %q — a clinic that predates the switch was left switched on",
			got, migrations.DefaultAIMode)
	}
}

// The backfill writes one column. Everything else on the row is somebody's
// configuration, and a migration that reached for app.Save on a record it
// did not author is how a stale in-memory value overwrites it.
func TestTheBackfillLeavesTheRestOfTheSettingsAlone(t *testing.T) {
	app := newAppBefore(t, theAIModeMigration)

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":           "clinic-already-running",
		"timing_minutes":    30,
		"is_active":         false,
		"template_name":     "recordatorio",
		"template_language": "es",
		"timezone":          "Europe/Madrid",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	record, err := app.FindRecordById(migrations.BusinessSettingsCollection, id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}

	if got := record.GetInt("timing_minutes"); got != 30 {
		t.Errorf("timing_minutes is %d, want 30", got)
	}
	if record.GetBool("is_active") {
		t.Error("is_active came back true; a clinic that had switched reminders off had them switched back on")
	}
	for field, want := range map[string]string{
		"user_id":           "clinic-already-running",
		"template_name":     "recordatorio",
		"template_language": "es",
		"timezone":          "Europe/Madrid",
	} {
		if got := record.GetString(field); got != want {
			t.Errorf("%s is %q, want %q", field, got, want)
		}
	}
}

// The other half of the same promise: a clinic that registers after this
// ticket and saves settings without naming a mode is off too. The backfill
// and the field default have to agree, or which of the two a clinic got
// would depend on when it signed up.
func TestASettingsRowSavedWithoutAModeStartsOff(t *testing.T) {
	app := newAppBefore(t, theAIModeMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":        "clinic-registered-today",
		"timing_minutes": 1440,
		"is_active":      true,
	})

	record, err := app.FindRecordById(migrations.BusinessSettingsCollection, id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}

	if got := record.GetString(migrations.AIModeField); got != migrations.DefaultAIMode {
		t.Errorf("a row saved without a mode has ai_mode %q, want %q", got, migrations.DefaultAIMode)
	}
}

// A clinic that has deliberately switched the assistant on keeps it on. The
// backfill has to touch the rows that have no mode and no others, which is
// the difference between "fill in the blanks" and "reset everybody".
func TestRunningTheMigrationsAgainDoesNotSwitchAnybodyBackOff(t *testing.T) {
	app := newAppBefore(t, theAIModeMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":              "clinic-on-autopilot",
		"timing_minutes":       1440,
		"is_active":            true,
		migrations.AIModeField: "autopilot",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations a second time: %v", err)
	}

	record, err := app.FindRecordById(migrations.BusinessSettingsCollection, id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}
	if got := record.GetString(migrations.AIModeField); got != "autopilot" {
		t.Errorf("ai_mode is %q, want %q", got, "autopilot")
	}
}

func TestRollingTheSwitchBackRemovesTheFieldAndKeepsTheSettings(t *testing.T) {
	app := newAppBefore(t, theAIModeMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":        "clinic-already-running",
		"timing_minutes": 720,
		"is_active":      true,
	})

	full := core.MigrationsList{}
	full.Copy(core.SystemMigrations)
	full.Copy(core.AppMigrations)

	if _, err := core.NewMigrationsRunner(app, full).Down(migrationsFrom(theAIModeMigration)); err != nil {
		t.Fatalf("revert the switch: %v", err)
	}

	// The down is what a rollback of the deploy carrying this runs, and the
	// reminders those clinics have configured are not this ticket's to lose.
	record, err := app.FindRecordById(migrations.BusinessSettingsCollection, id)
	if err != nil {
		t.Fatalf("the settings row did not survive the rollback: %v", err)
	}
	if got := record.GetInt("timing_minutes"); got != 720 {
		t.Errorf("timing_minutes is %d, want 720", got)
	}

	collection, err := app.FindCollectionByNameOrId(migrations.BusinessSettingsCollection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}
	if collection.Fields.GetByName(migrations.AIModeField) != nil {
		t.Error("ai_mode is still on the collection after the rollback")
	}
}
