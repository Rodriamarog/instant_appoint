package migrations_test

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/migrations"
)

// theConversationIndexMigration is the file that makes one appointment per
// conversation a rule the database keeps.
const theConversationIndexMigration = "1793232011_one_appointment_per_conversation.go"

const conversationIndexName = "idx_appointments_conversation"

// A clinic's existing appointments all carry an empty conversation_id,
// because until this ticket nothing wrote one. A unique index that did not
// exclude them would refuse the second of them outright, and the migration
// adding it would fail on every database that has ever had two appointments
// in it.
func TestTheIndexIsAddedToADatabaseFullOfAppointmentsBookedAtTheDesk(t *testing.T) {
	app := newAppBefore(t, theConversationIndexMigration)

	for _, name := range []string{"Ana Ruiz", "Beto Salas", "Carla Díaz"} {
		saveUnder(t, app, migrations.AppointmentsCollection, map[string]any{
			"user_id":     "clinic-already-running",
			"client_name": name,
			"start_time":  "2026-10-30T22:00:00.000Z",
			"end_time":    "2026-10-30T23:00:00.000Z",
		})
	}

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	if got := len(appointmentsIn(t, app)); got != 3 {
		t.Errorf("%d appointments survived the migration, want all three", got)
	}
	if indexes := indexNamesOn(t, app, migrations.AppointmentsCollection); !slices.Contains(indexes, conversationIndexName) {
		t.Errorf("the index is missing after the migration: %v", indexes)
	}
}

// Reverting has to take the index with it. An index left behind by a Down
// that only looked like it worked is invisible until the next deploy tries
// to add it again and fails on the duplicate name.
func TestRevertingTakesTheIndexAway(t *testing.T) {
	app := newApp(t)

	full := core.MigrationsList{}
	full.Copy(core.SystemMigrations)
	full.Copy(core.AppMigrations)
	// Counted rather than written down. Down takes a number of migrations,
	// so a literal here would quietly revert whatever the newest migration
	// happens to be and keep passing.
	if _, err := core.NewMigrationsRunner(app, full).Down(migrationsFrom(theConversationIndexMigration)); err != nil {
		t.Fatalf("revert the index migration: %v", err)
	}

	if indexes := indexNamesOn(t, app, migrations.AppointmentsCollection); slices.Contains(indexes, conversationIndexName) {
		t.Errorf("the index is still there after reverting: %v", indexes)
	}
}

// appointmentsIn reads every appointment, by the name the collection has
// after the rename.
func appointmentsIn(t *testing.T, app core.App) []*core.Record {
	t.Helper()

	records, err := app.FindRecordsByFilter(migrations.AppointmentsCollection, "1=1", "", 0, 0)
	if err != nil {
		t.Fatalf("read the appointments: %v", err)
	}

	return records
}
