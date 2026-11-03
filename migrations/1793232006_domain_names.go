package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// The two collections under the names the business uses for them. Each
// replaces a name inherited from the Next.js stack that said something the
// app does not mean:
//
//   - calendar_events held appointments, and nothing else in this repo ever
//     called them events.
//   - its title column held the client's name — the {{1}} the customer reads
//     as their own — under a word that invites a treatment name instead.
//   - reminder_settings also holds the business timezone, which is what the
//     dashboard needs to know where "today" starts, so nobody looking for the
//     timezone would think to open it.
//
// The names are exported and the old ones are not, so there is one spelling
// of each in the repo rather than one per caller.
const (
	AppointmentsCollection     = "appointments"
	BusinessSettingsCollection = "business_settings"
)

// The column, before and after. Both spellings are named rather than written
// out at each of the four call sites below: the up and the down have to agree
// on them exactly, and a typo in only one direction is invisible until
// somebody rolls a database back.
const (
	clientNameField = "client_name"
	oldTitleField   = "title"
)

func init() {
	m.Register(func(app core.App) error {
		if err := renameCollection(app, rename{
			id:        calendarEventsID,
			name:      AppointmentsCollection,
			fieldFrom: oldTitleField,
			fieldTo:   clientNameField,
			indexes: []string{
				"CREATE INDEX `idx_appointments_user_start` ON `appointments` (`user_id`, `start_time`)",
				"CREATE INDEX `idx_appointments_start` ON `appointments` (`start_time`)",
			},
		}); err != nil {
			return err
		}

		return renameCollection(app, rename{
			id:   reminderSettingsID,
			name: BusinessSettingsCollection,
			indexes: []string{
				"CREATE UNIQUE INDEX `idx_business_settings_user_id` ON `business_settings` (`user_id`)",
			},
		})
	}, func(app core.App) error {
		if err := renameCollection(app, rename{
			id:        calendarEventsID,
			name:      calendarEventsCollection,
			fieldFrom: clientNameField,
			fieldTo:   oldTitleField,
			indexes: []string{
				"CREATE INDEX `idx_calendar_events_user_start` ON `calendar_events` (`user_id`, `start_time`)",
				"CREATE INDEX `idx_calendar_events_start` ON `calendar_events` (`start_time`)",
			},
		}); err != nil {
			return err
		}

		return renameCollection(app, rename{
			id:   reminderSettingsID,
			name: reminderSettingsCollection,
			indexes: []string{
				"CREATE UNIQUE INDEX `idx_reminder_settings_user_id` ON `reminder_settings` (`user_id`)",
			},
		})
	})
}

// rename is one collection's new name, optionally one field's, and the index
// statements that have to be restated because they spell the table name out.
type rename struct {
	id        string
	name      string
	fieldFrom string
	fieldTo   string
	indexes   []string
}

// renameCollection renames in place, by id.
//
// By id, because the id is the thing this app pins and a database carried
// over from the Next.js stack is matched by: looking the collection up by
// either name would work in one direction only, and this function runs in
// both.
//
// In place, because the rows have to survive. PocketBase decides between an
// ALTER TABLE ... RENAME and a drop-plus-create by comparing the saved
// collection against the stored one field by *id*, so mutating the loaded
// field's name carries the values over while a fresh FieldsList.Add of a
// field named client_name would drop the column and add an empty one.
//
// It is also why the indexes are handed in: PocketBase rewrites the table
// name inside index DDL on save but leaves the index's own name alone, so a
// renamed collection would otherwise keep indexes called after a collection
// that no longer exists.
func renameCollection(app core.App, to rename) error {
	collection, err := app.FindCollectionByNameOrId(to.id)
	if err != nil {
		return nil // already gone
	}

	collection.Name = to.name

	if to.fieldFrom != "" {
		if field := collection.Fields.GetByName(to.fieldFrom); field != nil {
			field.SetName(to.fieldTo)
		}
	}

	collection.Indexes = types.JSONArray[string](to.indexes)

	return app.Save(collection)
}
