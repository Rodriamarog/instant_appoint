package migrations_test

import (
	"slices"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// The rename is the one migration in this repo that has to act on data it did
// not create, so every test below writes rows under the old names first and
// only then lets the rename run. Asserting against a database that was built
// by the current code would prove nothing: a create-and-forget migration
// passes that just as happily as one that carries the rows over.

// theRenameMigration is named rather than derived. PocketBase orders
// migrations by filename, so "every migration before the rename" is a
// filename comparison — the same ordering the runner itself applies.
const theRenameMigration = "1793232006_domain_names.go"

// newAppBeforeTheRename returns a bootstrapped app whose database is in the
// state it was in before this ticket: calendar_events with a title column,
// and reminder_settings.
func newAppBeforeTheRename(t *testing.T) core.App {
	t.Helper()

	return newAppBefore(t, theRenameMigration)
}

// newAppBefore returns a bootstrapped app with every migration applied except
// the one named and everything after it, so a test can write the rows a real
// database would already be holding and only then let that migration run.
//
// The cut is made on the filename because that is the ordering the migration
// runner itself applies: deriving "before" any other way would let the test
// and the runner disagree about what it means.
func newAppBefore(t *testing.T, migrationFile string) core.App {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		if err := app.ResetBootstrapState(); err != nil {
			t.Errorf("ResetBootstrapState: %v", err)
		}
	})

	earlier := core.MigrationsList{}
	earlier.Copy(core.SystemMigrations)
	for _, migration := range core.AppMigrations.Items() {
		if migration.File >= migrationFile {
			continue
		}
		earlier.Register(migration.Up, migration.Down, migration.File)
	}

	if _, err := core.NewMigrationsRunner(app, earlier).Up(); err != nil {
		t.Fatalf("apply the migrations before %s: %v", migrationFile, err)
	}

	return app
}

// migrationsFrom counts the migrations at and after the named one, which is
// the number Down() has to be given to revert it.
//
// Down takes a count of migrations, not a ticket, so the count is derived
// rather than written down: every migration added after the one under test
// would otherwise silently turn "revert this" into "revert the newest thing
// and assert against this anyway", which passes for exactly as long as the
// newest thing happens not to touch the same collections.
func migrationsFrom(migrationFile string) int {
	count := 0
	for _, migration := range core.AppMigrations.Items() {
		if migration.File >= migrationFile {
			count++
		}
	}

	return count
}

// saveUnder writes a row directly into a collection by name, which is how a
// fixture gets into a schema the current code no longer has names for.
func saveUnder(t *testing.T, app core.App, collection string, values map[string]any) string {
	t.Helper()

	target, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId(%s): %v", collection, err)
	}

	record := core.NewRecord(target)
	for key, value := range values {
		record.Set(key, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatalf("save into %s: %v", collection, err)
	}

	return record.Id
}

// indexNamesOn reports the indexes SQLite actually holds for a table, which
// is the only way to see that index DDL naming the old table was rebuilt
// rather than silently dropped.
func indexNamesOn(t *testing.T, app core.App, table string) []string {
	t.Helper()

	var rows []struct {
		Name string `db:"name"`
	}

	err := app.DB().
		NewQuery("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = {:table}").
		Bind(dbx.Params{"table": table}).
		All(&rows)
	if err != nil {
		t.Fatalf("read the indexes on %s: %v", table, err)
	}

	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}

	return names
}

func TestADatabaseWrittenBeforeTheRenameKeepsItsAppointments(t *testing.T) {
	app := newAppBeforeTheRename(t)

	id := saveUnder(t, app, "calendar_events", map[string]any{
		"user_id":      "carried-over-owner",
		"title":        "Ana Lopez",
		"client_phone": "5216641234567",
		"notes":        "Hello, just a reminder about your appointment tomorrow.",
		"start_time":   "2026-11-02T17:00:00.000Z",
		"end_time":     "2026-11-02T18:00:00.000Z",
		"location":     "Suite 2",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	record, err := app.FindRecordById("appointments", id)
	if err != nil {
		t.Fatalf("the appointment written before the rename is gone: %v", err)
	}

	// client_name is the one the rename has to carry over by value. A
	// dropped-and-re-added column would leave it empty here and the reminder
	// would greet the customer by nobody's name.
	if got := record.GetString("client_name"); got != "Ana Lopez" {
		t.Errorf("client_name is %q, want %q — the title column was not renamed in place", got, "Ana Lopez")
	}

	for field, want := range map[string]string{
		"user_id":      "carried-over-owner",
		"client_phone": "5216641234567",
		"notes":        "Hello, just a reminder about your appointment tomorrow.",
		"start_time":   "2026-11-02T17:00:00.000Z",
		"end_time":     "2026-11-02T18:00:00.000Z",
		"location":     "Suite 2",
	} {
		if got := record.GetString(field); got != want {
			t.Errorf("%s is %q, want %q", field, got, want)
		}
	}

	if _, err := app.FindCollectionByNameOrId("calendar_events"); err == nil {
		t.Error("calendar_events still resolves; the collection was copied rather than renamed")
	}
}

func TestADatabaseWrittenBeforeTheRenameKeepsItsSettings(t *testing.T) {
	app := newAppBeforeTheRename(t)

	id := saveUnder(t, app, "reminder_settings", map[string]any{
		"user_id":           "carried-over-owner",
		"timing_minutes":    120,
		"is_active":         true,
		"template_name":     "appointment_reminder",
		"template_language": "es",
		"timezone":          "America/Hermosillo",
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	record, err := app.FindRecordById("business_settings", id)
	if err != nil {
		t.Fatalf("the settings row written before the rename is gone: %v", err)
	}

	if got := record.GetInt("timing_minutes"); got != 120 {
		t.Errorf("timing_minutes is %d, want 120", got)
	}
	if !record.GetBool("is_active") {
		t.Error("is_active came back false; reminders would have switched themselves off")
	}
	for field, want := range map[string]string{
		"user_id":           "carried-over-owner",
		"template_name":     "appointment_reminder",
		"template_language": "es",
		"timezone":          "America/Hermosillo",
	} {
		if got := record.GetString(field); got != want {
			t.Errorf("%s is %q, want %q", field, got, want)
		}
	}

	if _, err := app.FindCollectionByNameOrId("reminder_settings"); err == nil {
		t.Error("reminder_settings still resolves; the collection was copied rather than renamed")
	}
}

func TestTheRenamedCollectionsKeepTheirPinnedIds(t *testing.T) {
	app := newAppBeforeTheRename(t)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	// The ids are what a database carried over from the Next.js stack is
	// matched by. A rename that minted new ones would leave that database
	// with two of each collection, one of them holding all the rows.
	for name, want := range map[string]string{
		"appointments":      "pbc_2544738948",
		"business_settings": "pbc_reminder_settings",
	} {
		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("FindCollectionByNameOrId(%s): %v", name, err)
		}
		if collection.Id != want {
			t.Errorf("%s has id %q, want the pinned %q", name, collection.Id, want)
		}
	}
}

func TestTheRenamedTablesCarryIndexesNamedAfterThem(t *testing.T) {
	app := newAppBeforeTheRename(t)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	// Index DDL embeds the table name, so the rename had to restate it. If it
	// had not, the old statements would name a table that no longer exists
	// and the queries the calendar and the scheduler run every minute would
	// fall back to a scan with nothing to say about it.
	for table, want := range map[string][]string{
		"appointments": {
			"idx_appointments_user_start",
			"idx_appointments_start",
		},
		"business_settings": {
			"idx_business_settings_user_id",
		},
	} {
		got := indexNamesOn(t, app, table)
		for _, name := range want {
			if !slices.Contains(got, name) {
				t.Errorf("%s has indexes %v, missing %q", table, got, name)
			}
		}
		for _, name := range got {
			if name == "idx_calendar_events_user_start" ||
				name == "idx_calendar_events_start" ||
				name == "idx_reminder_settings_user_id" {
				t.Errorf("%s still carries %q, an index named after the old collection", table, name)
			}
		}
	}
}

func TestRollingTheRenameBackRestoresTheOldNamesAndTheirRows(t *testing.T) {
	app := newAppBeforeTheRename(t)

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	id := saveUnder(t, app, "appointments", map[string]any{
		"user_id":     "carried-over-owner",
		"client_name": "Ana Lopez",
		"start_time":  "2026-11-02T17:00:00.000Z",
		"end_time":    "2026-11-02T18:00:00.000Z",
	})

	full := core.MigrationsList{}
	full.Copy(core.SystemMigrations)
	full.Copy(core.AppMigrations)
	if _, err := core.NewMigrationsRunner(app, full).Down(migrationsFrom(theRenameMigration)); err != nil {
		t.Fatalf("revert the rename: %v", err)
	}

	// The down is what somebody reaches for when the deploy carrying this
	// rename has to come off, so it has to be the rename run backwards and
	// not a drop: the appointments booked since are in here.
	record, err := app.FindRecordById("calendar_events", id)
	if err != nil {
		t.Fatalf("the appointment did not survive the rollback: %v", err)
	}
	if got := record.GetString("title"); got != "Ana Lopez" {
		t.Errorf("title is %q, want %q", got, "Ana Lopez")
	}

	if _, err := app.FindCollectionByNameOrId("business_settings"); err == nil {
		t.Error("business_settings still resolves after the rollback")
	}
}
