package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// LanguageField is the language the patient is written to in, on the
// appointment rather than on the business.
//
// A Tijuana clinic treats people from both sides of the border, and until now
// every reminder it sent went out in the single variant configured in
// business_settings.template_language — so half the list was reminded in a
// language they do not read. The prefix on the phone number is the right
// guess most of the time and wrong often enough to matter (a Mexican patient
// who prefers English is unremarkable here), which is why this is a stored
// column staff can change and not something derived at send time.
//
// It holds Meta's own variant code — "es", "en_US" — and not a language name
// of this app's invention. That is the exact value a send names, so there is
// one vocabulary here rather than two that can drift, and it is the same
// spelling business_settings.template_language already uses, which is what
// makes the backfill below a copy rather than a translation.
const LanguageField = "language"

func init() {
	m.Register(func(app core.App) error {
		// By the pinned ids, not by name: these two collections have been
		// renamed once already, and the ids are what a database carried over
		// from the Next.js stack is matched by.
		appointments, err := app.FindCollectionByNameOrId(calendarEventsID)
		if err != nil {
			return err
		}
		settings, err := app.FindCollectionByNameOrId(reminderSettingsID)
		if err != nil {
			return err
		}

		// AutogeneratePattern is PocketBase's only create-time default, and a
		// regex with no alternation or repetition generates exactly itself.
		// It is what stops a writer that has never heard of this column —
		// an older build of the SPA, a seed, the webhook — booking an
		// appointment with an empty language, which the reminder job reads
		// as one it cannot write a date for and holds back.
		appointments.Fields.Add(&core.TextField{
			Name:                LanguageField,
			Max:                 10,
			AutogeneratePattern: DefaultTemplateLanguage,
		})

		if err := app.Save(appointments); err != nil {
			return err
		}

		// Every appointment already in the database was being reminded about
		// in its owner's configured variant, so that is what each one is
		// backfilled with — per owner, not one default for everybody. A
		// clinic whose template is approved in en_US would otherwise wake up
		// to a calendar that had quietly switched to Spanish.
		//
		// COALESCE over NULLIF covers the owner with no settings row at all,
		// where the subquery returns NULL, and the one whose row holds an
		// empty variant. Both become the same default a new row is born
		// with, because an empty language is a reminder that stops going
		// out.
		//
		// Written as SQL rather than a record save per row because every
		// other column on those rows is a clinic's own data: this touches
		// the one column it is responsible for, bumps no `updated` stamp and
		// revalidates nothing it did not write. Both table names come from
		// the collections just loaded by id, so it still finds them after
		// the rename that has already happened to both.
		backfill := fmt.Sprintf(`
			UPDATE {{%[1]s}}
			   SET [[%[3]s]] = COALESCE(NULLIF((
			           SELECT [[template_language]] FROM {{%[2]s}}
			            WHERE {{%[2]s}}.[[user_id]] = {{%[1]s}}.[[user_id]]
			       ), ''), {:fallback})
			 WHERE [[%[3]s]] = '' OR [[%[3]s]] IS NULL
		`, appointments.Name, settings.Name, LanguageField)

		_, err = app.DB().NewQuery(backfill).
			Bind(dbx.Params{"fallback": DefaultTemplateLanguage}).
			Execute()

		return err
	}, func(app core.App) error {
		appointments, err := app.FindCollectionByNameOrId(calendarEventsID)
		if err != nil {
			return nil // already gone
		}

		appointments.Fields.RemoveByName(LanguageField)

		return app.Save(appointments)
	})
}
