package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/Rodriamarog/instant_appoint/internal/appointments"
)

// Whether an appointment is going to happen, and who last said so.
//
// Until now an appointment was a slot in a calendar and nothing more, so a
// clinic could not tell a confirmed patient from one who has gone quiet, and
// a cancellation could only be expressed by deleting the booking — which
// takes the reason, the history and the phone number with it.
//
// The five columns are three separate pieces of information:
//
//   - status, where the booking stands.
//   - status_set_by and status_set_at, how it got there. "Who said so" is the
//     first thing a receptionist asks of a screen that says cancelled, and
//     staff, the patient and the AI are three very different answers.
//   - created_by and conversation_id, where it came from. An appointment the
//     assistant wrote out of a WhatsApp thread has to be traceable back to
//     that thread, or nobody can check it.
//
// Free text with a create-time default rather than a select field, for one
// reason: a select has no default, so a caller that does not set status would
// store an empty one — and an empty status is an appointment no reminder will
// ever go out for, with nothing logged and nothing to see. The vocabulary
// lives in internal/appointments and is enforced by the code that writes it.
const (
	statusField         = "status"
	statusSetByField    = "status_set_by"
	statusSetAtField    = "status_set_at"
	createdByField      = "created_by"
	conversationIDField = "conversation_id"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId(calendarEventsID)
		if err != nil {
			return err
		}

		collection.Fields.Add(
			// The pattern generates exactly itself — it has no alternation
			// and no repetition — which is PocketBase's only way to give a
			// field a create-time default.
			&core.TextField{
				Name:                statusField,
				Max:                 20,
				AutogeneratePattern: string(appointments.Pending),
			},

			&core.TextField{Name: statusSetByField, Max: 20},

			// ISO-8601 as internal/isotime writes it, text like every other
			// timestamp in this app: the fixed width is what makes a string
			// comparison agree with chronological order.
			&core.TextField{Name: statusSetAtField, Max: 50},

			&core.TextField{Name: createdByField, Max: 20},

			// The conversations row this appointment came out of, empty when
			// a human booked it at the desk. A plain id rather than a
			// relation field, matching whatsapp_messages.conversation_id —
			// a relation would cascade, and deleting a conversation must not
			// take the clinic's appointments with it.
			&core.TextField{Name: conversationIDField, Max: 255},
		)

		if err := app.Save(collection); err != nil {
			return err
		}

		// Every appointment already in the database was booked by somebody
		// at the business, in this app, before there was any other way to
		// book one. Pending is what that is, and leaving them blank would
		// switch off every reminder a clinic currently depends on.
		//
		// created is PocketBase's own column, written "2006-01-02 15:04:05.000Z";
		// swapping the space for a T is the same instant in the shape the
		// rest of the app stores timestamps in.
		_, err = app.DB().NewQuery(`
			UPDATE appointments
			   SET status = {:status},
			       status_set_by = {:actor},
			       created_by = {:actor},
			       status_set_at = replace(created, ' ', 'T')
			 WHERE status IS NULL OR status = ''
		`).Bind(dbx.Params{
			"status": string(appointments.Pending),
			"actor":  string(appointments.Staff),
		}).Execute()

		return err
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId(calendarEventsID)
		if err != nil {
			return nil // already gone
		}

		for _, field := range []string{
			statusField, statusSetByField, statusSetAtField,
			createdByField, conversationIDField,
		} {
			collection.Fields.RemoveByName(field)
		}

		return app.Save(collection)
	})
}
