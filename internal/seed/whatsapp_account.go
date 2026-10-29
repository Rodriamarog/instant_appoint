package seed

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/whatsapp"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// The fixed identifiers of the seeded WhatsApp connection.
//
// A real one can only be created by a human completing Meta's Embedded Signup
// in a browser, so development and every automated check would otherwise have
// no connected account to work against. These stand in for one.
const (
	// TestWABAID and TestPhoneNumberID are plausible shapes — Meta's ids are
	// long digit strings — but they belong to no real account.
	TestWABAID        = "100000000000001"
	TestPhoneNumberID = "200000000000002"

	testVerifiedName = "Dev Clinic"
	testPhoneNumber  = "5216641234567"

	// The token says what it is in its own value. Anything that logs it, or
	// anyone who finds it in a database, can see at a glance that it is not a
	// credential worth protecting or worth trying against Meta.
	testAccessToken = "FAKE-SEEDED-TOKEN-not-a-real-meta-credential"
)

// whatsAppAccount ensures the test user has exactly one connected Cloud API
// account.
//
// It resolves the existing row the same way the signup handler does, so a
// re-run converges onto that row instead of adding a second one.
func whatsAppAccount(app core.App, result *Result) error {
	result.WhatsAppWABAID = TestWABAID
	result.WhatsAppPhoneNumberID = TestPhoneNumberID
	result.WhatsAppVerifiedName = testVerifiedName

	account, err := whatsapp.AccountFor(app, result.UserID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		collection, err := app.FindCollectionByNameOrId(migrations.WhatsAppAccountsCollection)
		if err != nil {
			return err
		}
		account = core.NewRecord(collection)
		account.Set("user_id", result.UserID)
		account.Set("session_id", "business_"+result.UserID)
	case err != nil:
		return err
	}

	// Set on every run so a row left half-edited in the admin UI is repaired
	// rather than quietly kept.
	account.Set("account_type", whatsapp.BusinessAPIAccount)
	account.Set("waba_id", TestWABAID)
	account.Set("phone_number_id", TestPhoneNumberID)
	account.Set("access_token", testAccessToken)
	account.Set("verified_name", testVerifiedName)
	// Written by the retired QR service only, so a real Cloud API account
	// leaves it empty and the page falls back to the phone number id. Seeded
	// here so the page's normal rendering path is the one being looked at.
	account.Set("phone_number", testPhoneNumber)
	account.Set("status", "connected")
	account.Set("is_active", true)
	account.Set("disconnected_at", "")

	if err := app.Save(account); err != nil {
		return err
	}

	result.WhatsAppAccountID = account.Id

	return nil
}
