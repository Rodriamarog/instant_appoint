package seed_test

import (
	"testing"

	"github.com/pocketbase/dbx"

	"github.com/Rodriamarog/instant_appoint/internal/seed"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// The dev clinic answers its patients.
//
// #26 made every clinic start with the assistant off, deliberately: switching
// it on is the moment a business takes responsibility for what it says. The
// seeded clinic is not a business, and leaving it off meant a freshly seeded
// database produced no AI reply at all — the inbound path looked broken to
// anyone trying it for the first time, with nothing on screen to say why.
func TestApplyTurnsTheSeededClinicsAssistantOn(t *testing.T) {
	app := newApp(t)

	result, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	settings, err := app.FindFirstRecordByFilter(
		migrations.BusinessSettingsCollection,
		"user_id = {:userID}", dbx.Params{"userID": result.UserID},
	)
	if err != nil {
		t.Fatalf("the seeded clinic has no settings row: %v", err)
	}

	if got := settings.GetString("ai_mode"); got != "autopilot" {
		t.Errorf("ai_mode = %q, want autopilot so the dev loop answers a message", got)
	}
}

// The collection holds one row per person — the unique index says so — so a
// second run has to find the first row rather than write another.
func TestApplyDoesNotDuplicateTheSettingsRow(t *testing.T) {
	app := newApp(t)

	if _, err := seed.Apply(app); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if _, err := seed.Apply(app); err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	rows, err := app.FindAllRecords(migrations.BusinessSettingsCollection)
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("after two runs there are %d settings rows, want 1", len(rows))
	}
}

// A clinic's own choices are not the seed's to overwrite. Only the assistant
// is written on a re-run; the lead time somebody set while testing stays set.
func TestApplyLeavesSettingsSomebodyChangedAlone(t *testing.T) {
	app := newApp(t)

	result, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	settings, err := app.FindFirstRecordByFilter(
		migrations.BusinessSettingsCollection,
		"user_id = {:userID}", dbx.Params{"userID": result.UserID},
	)
	if err != nil {
		t.Fatalf("FindFirstRecordByFilter: %v", err)
	}
	settings.Set("timing_minutes", 30)
	settings.Set("template_name", "recordatorio_de_cita")
	if err := app.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := seed.Apply(app); err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	settings, err = app.FindFirstRecordByFilter(
		migrations.BusinessSettingsCollection,
		"user_id = {:userID}", dbx.Params{"userID": result.UserID},
	)
	if err != nil {
		t.Fatalf("FindFirstRecordByFilter: %v", err)
	}
	if got := settings.GetInt("timing_minutes"); got != 30 {
		t.Errorf("timing_minutes = %d, want the 30 somebody set", got)
	}
	if got := settings.GetString("template_name"); got != "recordatorio_de_cita" {
		t.Errorf("template_name = %q, want the one somebody chose", got)
	}
}
