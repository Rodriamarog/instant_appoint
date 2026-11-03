package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// AIModeField is the column on business_settings that says how much a clinic
// has let the assistant do. Before it there was no way to stop the assistant
// answering: it replied to every message under fifteen minutes old, for
// everybody, permanently.
//
// The three positions it holds are spelled out in internal/webhook, which is
// where the one decision that reads them lives. They are not shared through a
// constant because that package may not import this one — it may not import
// PocketBase at all, which is what keeps the reply decision answerable with
// no database. The spellings are pinned on both sides instead, and
// internal/pbstore has the test that they still agree.
const AIModeField = "ai_mode"

// DefaultAIMode is the position a clinic has until it says otherwise.
//
// Off, because nobody's patients should be answered by a bot their clinic
// never knowingly switched on, and because switching it on deliberately is
// the moment the clinic takes responsibility for what it says.
const DefaultAIMode = "off"

func init() {
	m.Register(func(app core.App) error {
		// By the pinned id, not by name. The id is what a database carried
		// over from the Next.js stack is matched by, and it is also the one
		// handle that does not move: this collection has been renamed once
		// already, which is why the constant still reads reminderSettingsID
		// while the collection is called business_settings.
		settings, err := app.FindCollectionByNameOrId(reminderSettingsID)
		if err != nil {
			return nil // already gone
		}

		// AutogeneratePattern is PocketBase's only create-time default, and a
		// regex with no alternation or repetition generates exactly itself.
		// It covers the rows written from here on; the UPDATE below covers
		// the rows that are already there, and the two have to name the same
		// position or which one a clinic got would depend on the day it
		// registered.
		settings.Fields.Add(&core.TextField{
			Name:                AIModeField,
			Max:                 20,
			AutogeneratePattern: DefaultAIMode,
		})

		if err := app.Save(settings); err != nil {
			return err
		}

		// Adding a column leaves every existing row holding "", which the
		// reply gate already reads as "not autopilot" — but "" is not one of
		// the three positions, so the settings control would show a clinic
		// no position selected and the first save would be the clinic
		// picking one blind. The state has to be said, not implied.
		//
		// Written as SQL rather than a record save per row because every
		// other column on those rows is somebody's configuration: this
		// touches the one column it is responsible for, bumps no `updated`
		// stamp, and revalidates nothing it did not write.
		//
		// The table is named from the collection just loaded by id rather
		// than spelled out, so this still finds it after the rename that has
		// already happened to it once.
		backfill := fmt.Sprintf(
			"UPDATE {{%s}} SET [[%s]] = {:mode} WHERE [[%s]] = '' OR [[%s]] IS NULL",
			settings.Name, AIModeField, AIModeField, AIModeField,
		)

		_, err = app.DB().NewQuery(backfill).Bind(dbx.Params{"mode": DefaultAIMode}).Execute()

		return err
	}, func(app core.App) error {
		settings, err := app.FindCollectionByNameOrId(reminderSettingsID)
		if err != nil {
			return nil // already gone
		}

		settings.Fields.RemoveByName(AIModeField)

		return app.Save(settings)
	})
}
