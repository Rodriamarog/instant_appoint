package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// One appointment per conversation, enforced by the database.
//
// The assistant reads a whole WhatsApp thread on every message that arrives
// in it, so the agreement reached in the first message is still there in the
// fortieth. The handler checks for an appointment already written from the
// conversation before it writes one, but a check followed by a write is not
// one operation: two deliveries of the same thread can be in flight at once,
// on two servers, and neither can see the other. This index can.
//
// It is also the lookup that check runs. Without it, every inbound message
// to a clinic with the assistant switched on scans the whole appointments
// table.
//
// Partial, on a non-empty conversation_id, for the reason that makes this
// safe at all: every appointment booked at the desk carries an empty one,
// and an index that did not exclude them would stop a clinic booking a
// second appointment ever again.
const appointmentConversationIndex = "CREATE UNIQUE INDEX `idx_appointments_conversation` " +
	"ON `appointments` (`conversation_id`) WHERE `conversation_id` != ''"

func init() {
	m.Register(func(app core.App) error {
		// By the pinned id, not by name: the collection has been renamed
		// once already, which is why the constant still reads
		// calendarEventsID while the table is called appointments.
		collection, err := app.FindCollectionByNameOrId(calendarEventsID)
		if err != nil {
			return err
		}

		collection.Indexes = append(collection.Indexes, appointmentConversationIndex)

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId(calendarEventsID)
		if err != nil {
			return nil // already gone
		}

		collection.Indexes = slices.DeleteFunc(collection.Indexes, func(index string) bool {
			return index == appointmentConversationIndex
		})

		return app.Save(collection)
	})
}
