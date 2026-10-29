package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// ReminderSettingsCollection is the one row per person that says when their
// appointment reminders go out, and whether they go out at all. The id is
// pinned to the one the legacy JS migration used so a database carried over
// from the Next.js stack keeps its rows.
const (
	ReminderSettingsCollection = "reminder_settings"
	reminderSettingsID         = "pbc_reminder_settings"
)

// The defaults a row is born with. They are declared here, not only in the
// browser, because the scheduler in #9 reads stored rows and has to find a
// usable template and zone on every one of them — including a row written by
// something that never heard of the settings dialog.
//
// DefaultTimingMinutes is deliberately *not* one of them: it is the one
// setting the dialog always sends, and a reminder lead time silently
// defaulting to a day is a decision the person should make rather than
// inherit. It is exported so the SPA's fallback and the scheduler's agree.
const (
	DefaultTimingMinutes    = 1440
	DefaultTemplateLanguage = "es"
	DefaultTimezone         = "America/Tijuana"
)

// Reads and writes are both owner-scoped: the SPA reaches PocketBase directly
// and these rules are the whole of the access control. The plain field
// reference rather than @request.body.user_id is what makes the create rule
// reject a forged owner — on create PocketBase evaluates the rule against the
// record as it would be written.
const reminderSettingsOwner = "user_id = @request.auth.id"

func init() {
	m.Register(func(app core.App) error {
		settings := core.NewBaseCollection(ReminderSettingsCollection, reminderSettingsID)

		settings.Fields.Add(
			&core.TextField{Name: "user_id", Required: true, Max: 255},

			// How long before the appointment the reminder goes out.
			// Required on a number means non-zero, and a zero lead time —
			// "send it as the appointment starts" — is not a thing this app
			// offers, so the two agree.
			&core.NumberField{
				Name:     "timing_minutes",
				Required: true,
				OnlyInt:  true,
				Min:      types.Pointer(float64(1)),
			},

			// Required on a bool means "must be true" in PocketBase, which
			// would make switching reminders off unstorable. Left optional.
			&core.BoolField{Name: "is_active"},

			// The approved Meta template a reminder is sent as, and the
			// language variant of it. Reminders cannot be free text: outside
			// the 24-hour customer-service window only an approved template
			// is deliverable. #8 lists the names, #9 sends one.
			//
			// template_name has no default: there is no template name that is
			// right for everybody, and #9 has to be able to tell "not chosen
			// yet" from "chosen".
			&core.TextField{Name: "template_name", Max: 255},

			// AutogeneratePattern is PocketBase's only create-time default: a
			// regex with no alternation or repetition generates exactly
			// itself, and it fires only when the field is empty on a new
			// record (core/field_text.go:359). So a row created without these
			// two carries the defaults, whoever created it.
			&core.TextField{
				Name:                "template_language",
				Max:                 10,
				AutogeneratePattern: DefaultTemplateLanguage,
			},

			// The zone the date and time in a reminder are spelled out in.
			// Appointments are stored as UTC instants, so without this a
			// reminder would name an hour nobody recognises. A pure-Go binary
			// carries no zone database, which is why cmd/server blank-imports
			// time/tzdata.
			&core.TextField{
				Name:                "timezone",
				Max:                 64,
				AutogeneratePattern: DefaultTimezone,
			},

			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)

		settings.ListRule = types.Pointer(reminderSettingsOwner)
		settings.ViewRule = types.Pointer(reminderSettingsOwner)
		settings.CreateRule = types.Pointer(reminderSettingsOwner)
		settings.UpdateRule = types.Pointer(reminderSettingsOwner)
		settings.DeleteRule = types.Pointer(reminderSettingsOwner)

		// One row per person, enforced rather than assumed. Every reader —
		// the page, and the scheduler after it — resolves settings with a
		// first-match lookup, which would silently pick an arbitrary row once
		// a duplicate existed. The legacy schema had no index at all.
		settings.Indexes = types.JSONArray[string]{
			"CREATE UNIQUE INDEX `idx_reminder_settings_user_id` ON `reminder_settings` (`user_id`)",
		}

		return app.Save(settings)
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId(ReminderSettingsCollection)
		if err != nil {
			return nil // already gone
		}

		return app.Delete(settings)
	})
}
