package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// CalendarEventsCollection is the appointments table. The id is pinned to the
// one the legacy JS migration used so a database carried over from the Next.js
// stack keeps working, and so anything that references the collection by id
// resolves the same way in every environment.
//
// The name is exported for the same reason as the other four: a collection
// name spelled a second time somewhere else is a collection name that can be
// changed in one place and not the other.
const (
	CalendarEventsCollection = "calendar_events"
	calendarEventsID         = "pbc_2544738948"
)

// The SPA reads and writes calendar_events directly through the PocketBase
// SDK — there is no hand-written API layer left to check who is asking. These
// rules are therefore the whole of the access control.
//
// owner is deliberately the plain field reference rather than
// @request.body.user_id. On create PocketBase evaluates the rule against the
// record as it would be written (the submitted body loaded into a record,
// injected as a single-row CTE), so the plain form already rejects a forged
// user_id — and unlike the @request.body form it also sees values that arrive
// from a field default rather than from the client.
//
// A logged-out caller has @request.auth.id == "", which no stored user_id can
// equal because the field is required and non-empty, so the same expression
// closes the anonymous case too.
const calendarEventsOwner = "user_id = @request.auth.id"

func init() {
	m.Register(func(app core.App) error {
		events := core.NewBaseCollection(CalendarEventsCollection, calendarEventsID)

		// Every field is text, including the two timestamps. The app stores
		// ISO-8601 strings and the calendar window filters them with >= / <=,
		// which SQLite resolves as a byte-by-byte string comparison. A date
		// field would store a different shape (space-separated, not "T") and
		// silently change what every range query means.
		events.Fields.Add(
			&core.TextField{Name: "user_id", Required: true, Max: 255},
			&core.TextField{Name: "title", Required: true, Max: 255},
			&core.TextField{Name: "client_phone", Max: 32},
			&core.TextField{Name: "notes", Max: 2000},
			&core.TextField{Name: "start_time", Required: true, Max: 32},
			&core.TextField{Name: "end_time", Required: true, Max: 32},
			&core.TextField{Name: "location", Max: 255},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)

		events.ListRule = types.Pointer(calendarEventsOwner)
		events.ViewRule = types.Pointer(calendarEventsOwner)
		events.CreateRule = types.Pointer(calendarEventsOwner)
		events.UpdateRule = types.Pointer(calendarEventsOwner)
		events.DeleteRule = types.Pointer(calendarEventsOwner)

		// The calendar page runs one query per navigation: owner, then an
		// overlap test on the two timestamps, sorted by start_time. The
		// reminder scheduler will scan start_time across all users.
		events.Indexes = types.JSONArray[string]{
			"CREATE INDEX `idx_calendar_events_user_start` ON `calendar_events` (`user_id`, `start_time`)",
			"CREATE INDEX `idx_calendar_events_start` ON `calendar_events` (`start_time`)",
		}

		return app.Save(events)
	}, func(app core.App) error {
		events, err := app.FindCollectionByNameOrId(CalendarEventsCollection)
		if err != nil {
			return nil // already gone
		}

		return app.Delete(events)
	})
}
