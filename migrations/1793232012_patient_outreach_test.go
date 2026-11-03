package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/migrations"
)

// thePatientOutreachMigration is the file that adds the column. Everything
// below starts from the database as it was before it, because the claim
// under test is about rows the migration did not write.
const thePatientOutreachMigration = "1793232012_patient_outreach.go"

// A clinic already running has the assistant checking its appointments with
// patients today, and this ticket is a control over that, not a change to
// it. Switching the column on for new clinics is a field default; switching
// it on for the clinics already there is this, and only this, and nothing in
// a fresh database can tell the two apart.
func TestEveryClinicThatExistedBeforeThisTicketKeepsTheReceipt(t *testing.T) {
	app := newAppBefore(t, thePatientOutreachMigration)

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":              "clinic-already-running",
		"timing_minutes":       120,
		"is_active":            true,
		"template_name":        "appointment_reminder",
		"template_language":    "es",
		"timezone":             "America/Hermosillo",
		migrations.AIModeField: "observer",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	record, err := app.FindRecordById(migrations.BusinessSettingsCollection, id)
	if err != nil {
		t.Fatalf("the settings row written before the column is gone: %v", err)
	}

	// Spelled out rather than merely "not off": the control in Configuración
	// shows the stored word, and a row left at "" would show a clinic a
	// switch in neither position.
	got := record.GetString(migrations.PatientOutreachField)
	if got != migrations.DefaultPatientOutreach {
		t.Errorf("patient_outreach is %q, want %q — a clinic that predates the column lost its receipts",
			got, migrations.DefaultPatientOutreach)
	}
}

// The other half of the same promise, and the half the backfill cannot
// reach: a clinic that registers after this ticket and saves settings
// without naming a value is on too. The field default and the backfill have
// to agree, or which of the two a clinic got would depend on when it signed
// up.
func TestASettingsRowSavedWithoutAnAnswerStartsWithTheReceiptOn(t *testing.T) {
	app := newAppBefore(t, thePatientOutreachMigration)

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

	got := record.GetString(migrations.PatientOutreachField)
	if got != migrations.DefaultPatientOutreach {
		t.Errorf("a row saved without an answer has patient_outreach %q, want %q",
			got, migrations.DefaultPatientOutreach)
	}
}

// The backfill writes one column. Everything else on the row is somebody's
// configuration — the reminder settings above all, which this ticket
// promises not to touch — and a migration that reached for app.Save on a
// record it did not author is how a stale in-memory value overwrites it.
func TestTheBackfillLeavesTheRemindersAndTheModeAlone(t *testing.T) {
	app := newAppBefore(t, thePatientOutreachMigration)

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":              "clinic-already-running",
		"timing_minutes":       30,
		"is_active":            true,
		"template_name":        "recordatorio",
		"template_language":    "es",
		"timezone":             "Europe/Madrid",
		migrations.AIModeField: "autopilot",
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
	if !record.GetBool("is_active") {
		t.Error("is_active came back false; a clinic's reminders were switched off by a column about receipts")
	}
	for field, want := range map[string]string{
		"user_id":              "clinic-already-running",
		"template_name":        "recordatorio",
		"template_language":    "es",
		"timezone":             "Europe/Madrid",
		migrations.AIModeField: "autopilot",
	} {
		if got := record.GetString(field); got != want {
			t.Errorf("%s is %q, want %q", field, got, want)
		}
	}
}

// A clinic that has deliberately turned the receipt off keeps it off. The
// backfill has to touch the rows that have no answer and no others, which is
// the difference between "fill in the blanks" and "turn everybody back on".
func TestRunningTheMigrationsAgainDoesNotSwitchAnybodyBackOn(t *testing.T) {
	app := newAppBefore(t, thePatientOutreachMigration)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, migrations.BusinessSettingsCollection, map[string]any{
		"user_id":                       "clinic-that-wants-quiet",
		"timing_minutes":                1440,
		"is_active":                     true,
		migrations.PatientOutreachField: "off",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations a second time: %v", err)
	}

	record, err := app.FindRecordById(migrations.BusinessSettingsCollection, id)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}
	if got := record.GetString(migrations.PatientOutreachField); got != "off" {
		t.Errorf("patient_outreach is %q, want %q", got, "off")
	}
}

func TestRollingTheControlBackRemovesTheFieldAndKeepsTheSettings(t *testing.T) {
	app := newAppBefore(t, thePatientOutreachMigration)

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

	// Counted, never written down. Down takes a number of migrations, so a
	// literal here would revert whichever migration is newest and keep
	// passing for exactly as long as nothing newer touches this collection.
	runner := core.NewMigrationsRunner(app, full)
	if _, err := runner.Down(migrationsFrom(thePatientOutreachMigration)); err != nil {
		t.Fatalf("revert the control: %v", err)
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
	if collection.Fields.GetByName(migrations.PatientOutreachField) != nil {
		t.Error("patient_outreach is still on the collection after the rollback")
	}
}
