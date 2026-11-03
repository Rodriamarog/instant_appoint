package seed

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/migrations"
)

// businessSettings switches the seeded clinic's assistant on.
//
// Every other field is left to the column defaults on create and left alone
// on a re-run, so this fixture writes one value rather than resetting a
// configuration somebody was in the middle of trying out.
//
// The value is autopilot and not the off every real clinic starts with: off
// is a promise to the clinic's patients that a model will not answer them,
// and the seeded clinic has none. What it has instead is a developer sending
// a message to it to see whether the inbound path works, and with no settings
// row at all that produced silence indistinguishable from a broken webhook.
func businessSettings(app core.App, result *Result) error {
	settings, err := app.FindFirstRecordByFilter(
		migrations.BusinessSettingsCollection,
		"user_id = {:userID}", dbx.Params{"userID": result.UserID},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		collection, err := app.FindCollectionByNameOrId(migrations.BusinessSettingsCollection)
		if err != nil {
			return err
		}
		settings = core.NewRecord(collection)
		settings.Set("user_id", result.UserID)
		// The one field with no column default, because no lead time is
		// right for everybody. The seeded clinic still needs one, and this
		// is the same day-ahead the dialog offers as its own fallback.
		settings.Set("timing_minutes", migrations.DefaultTimingMinutes)
		settings.Set("is_active", true)

	case err != nil:
		return err
	}

	settings.Set("ai_mode", seededAIMode)

	return app.Save(settings)
}

// What the seeded clinic's assistant is allowed to do. The three words are a
// storage format shared with internal/webhook and the SPA; see
// migrations/1793232008_ai_mode.go.
const seededAIMode = "autopilot"
