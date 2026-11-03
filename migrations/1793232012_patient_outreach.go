package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// PatientOutreachField is the column on business_settings that says whether
// the assistant may send a patient a message of its own accord.
//
// It is a separate column from ai_mode rather than a fourth position on it
// because the two answer different questions. ai_mode says what the
// assistant may do with a conversation; this says whether it may start one.
// Folding them together is what produced the state this column exists to
// undo: a clinic in Observador was told the assistant "does not reply", and
// then got a creation receipt sent to its patients anyway.
//
// The two words it holds are spelled out in internal/webhook, where the one
// decision that reads them lives, for the same reason ai_mode's three are:
// that package may not import this one, or PocketBase at all. The spellings
// are pinned on both sides and internal/pbstore has the test that they
// still agree.
const PatientOutreachField = "patient_outreach"

// DefaultPatientOutreach is what a clinic has until it says otherwise.
//
// On — which is the opposite of ai_mode's default, and not an inconsistency.
// A clinic only ever reaches this setting by having already put the
// assistant in Observador or Piloto automático, which is to say it has
// already agreed to the assistant writing appointments into its calendar.
// The receipt is the one thing that puts such an appointment in front of
// the person who knows whether they really agreed to Tuesday at three.
// Starting it off would leave the write path running with nothing watching
// it, which is the worse of the two failures.
//
// It is also what every clinic already gets today, so nobody's patients
// stop or start hearing from the assistant because this column appeared.
const DefaultPatientOutreach = "on"

func init() {
	m.Register(func(app core.App) error {
		// By the pinned id, as everything that touches this collection
		// does: it has been renamed once already, which is why the
		// constant still reads reminderSettingsID while the collection is
		// called business_settings.
		settings, err := app.FindCollectionByNameOrId(reminderSettingsID)
		if err != nil {
			return nil // already gone
		}

		// AutogeneratePattern is PocketBase's only create-time default, and
		// a regex with no alternation or repetition generates exactly
		// itself. It covers rows written from here on; the UPDATE below
		// covers the rows already there. The two have to name the same
		// value or whether a clinic's patients get a receipt would depend
		// on the day it registered.
		settings.Fields.Add(&core.TextField{
			Name:                PatientOutreachField,
			Max:                 20,
			AutogeneratePattern: DefaultPatientOutreach,
		})

		if err := app.Save(settings); err != nil {
			return err
		}

		// Adding a column leaves every existing row holding "". The gate
		// reads that as on, so no clinic's behaviour would change either
		// way — but the control in Configuración shows the stored word, and
		// a row left empty would show a clinic a switch in neither
		// position. The state has to be said, not implied.
		//
		// SQL rather than a record save per row: every other column on
		// those rows is somebody's configuration, and this touches the one
		// column it is responsible for, bumps no `updated` stamp and
		// revalidates nothing it did not write.
		backfill := fmt.Sprintf(
			"UPDATE {{%s}} SET [[%s]] = {:outreach} WHERE [[%s]] = '' OR [[%s]] IS NULL",
			settings.Name, PatientOutreachField, PatientOutreachField, PatientOutreachField,
		)

		_, err = app.DB().NewQuery(backfill).
			Bind(dbx.Params{"outreach": DefaultPatientOutreach}).Execute()

		return err
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId(reminderSettingsID)
		if err != nil {
			return nil // already gone
		}

		settings.Fields.RemoveByName(PatientOutreachField)

		return app.Save(settings)
	})
}
