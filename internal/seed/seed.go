// Package seed creates the development fixtures the app cannot create for
// itself.
//
// The only one today is a test account. Registration sends a verification
// email and the login page refuses an unverified account, so without SMTP
// credentials — which only the project owner can supply — nobody can log in to
// a fresh database. Seeding writes the account straight to the collection with
// verified already set, bypassing mail entirely.
package seed

import (
	"github.com/pocketbase/pocketbase/core"
)

// The fixed credentials of the seeded account. They are deliberately constant:
// anything that logs in during development or in a test script can hard-code
// them.
const (
	TestEmail    = "dev@instantappoint.test"
	TestPassword = "devpassword123"

	testName         = "Dev Tester"
	testBusinessName = "Dev Clinic"
)

// Result describes what the seed left behind, in a form a script can read.
type Result struct {
	Email    string
	Password string
	UserID   string
}

// fixture applies one piece of development data and records what it made.
//
// Every fixture must be idempotent: seeding runs against databases that may
// already be half-seeded, and re-running must converge rather than fail. New
// fixtures (ticket #5 adds a connected WhatsApp account) are appended to
// fixtures below and get their own field on Result.
type fixture func(core.App, *Result) error

var fixtures = []fixture{
	testUser,
}

// Apply writes every development fixture, creating what is missing and
// correcting what has drifted.
func Apply(app core.App) (Result, error) {
	var result Result

	for _, f := range fixtures {
		if err := f(app, &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// testUser ensures the verified test account exists with the published
// credentials.
func testUser(app core.App, result *Result) error {
	result.Email = TestEmail
	result.Password = TestPassword

	user, err := app.FindAuthRecordByEmail("users", TestEmail)
	if err != nil {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		user = core.NewRecord(collection)
		user.SetEmail(TestEmail)
	}

	// Set on every run, not only on create: a run after someone changed the
	// password in the admin UI must restore the documented credentials, or the
	// command stops being a reliable way back in.
	user.SetPassword(TestPassword)
	user.SetVerified(true)
	user.SetEmailVisibility(true)
	user.Set("name", testName)
	user.Set("business_name", testBusinessName)

	if err := app.Save(user); err != nil {
		return err
	}

	result.UserID = user.Id

	return nil
}
