// Package whatsapp owns the server side of connecting a WhatsApp Business
// number through Meta's Embedded Signup.
//
// The browser never sees a Meta credential: it sends the authorization code
// the signup popup gave it, and everything from there — exchanging that code
// for an access token, working out which phone number was onboarded,
// subscribing the account to webhook events and storing the result — happens
// here, because the exchange needs the app secret.
package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// BusinessAPIAccount is the account_type of a Cloud API connection, as opposed
// to "web", which the retired QR flow wrote.
const BusinessAPIAccount = "business_api"

// ErrIncompleteSignup means the signup did not carry enough to act on.
var ErrIncompleteSignup = errors.New("whatsapp: the signup is missing its code or its WhatsApp Business account id")

// Signup is what the browser reports when Meta's Embedded Signup popup
// finishes.
type Signup struct {
	// Code is the one-time authorization code. Meta rejects a second use.
	Code string `json:"code"`

	// WABAID identifies the WhatsApp Business Account that was onboarded.
	WABAID string `json:"waba_id"`

	// PhoneNumberID may be empty. A business that keeps using the WhatsApp
	// Business app finishes with FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING and
	// Meta sends no number, so this is looked up from the WABA instead.
	PhoneNumberID string `json:"phone_number_id"`
}

// Connect completes an Embedded Signup for one person and returns the stored
// account.
//
// Nothing is written until Meta has accepted the code and the phone number is
// known, so a failed signup leaves no half-connected row behind. Two of the
// four Graph calls are advisory and only logged on failure — see below.
func Connect(ctx context.Context, app core.App, client *meta.Client, userID string, signup Signup) (*core.Record, error) {
	if signup.Code == "" || signup.WABAID == "" {
		return nil, ErrIncompleteSignup
	}

	accessToken, err := client.ExchangeCode(ctx, signup.Code)
	if err != nil {
		return nil, fmt.Errorf("exchange the signup code: %w", err)
	}

	phoneNumberID, err := client.ResolvePhoneNumberID(ctx, signup.WABAID, signup.PhoneNumberID, accessToken)
	if err != nil {
		return nil, fmt.Errorf("resolve the onboarded phone number: %w", err)
	}

	// Advisory: without the subscription Meta delivers no webhooks, so the
	// account is connected but deaf. It is still a connected account, the
	// subscription can be retried, and throwing away a token that can only be
	// exchanged once would be worse.
	if err := client.SubscribeApp(ctx, signup.WABAID, accessToken); err != nil {
		app.Logger().Warn("the WABA webhook subscription was not confirmed; inbound messages will not arrive until it is",
			"waba_id", signup.WABAID, "error", err)
	}

	// Advisory: the display name is a label on the page.
	verifiedName, err := client.PhoneNumberVerifiedName(ctx, phoneNumberID, accessToken)
	if err != nil {
		app.Logger().Warn("could not read the number's verified name from Meta",
			"phone_number_id", phoneNumberID, "error", err)
	}

	account, err := AccountFor(app, userID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		collection, err := app.FindCollectionByNameOrId(migrations.WhatsAppAccountsCollection)
		if err != nil {
			return nil, err
		}
		account = core.NewRecord(collection)
		account.Set("user_id", userID)
		account.Set("session_id", "business_"+userID)
	case err != nil:
		return nil, err
	}

	account.Set("account_type", BusinessAPIAccount)
	account.Set("waba_id", signup.WABAID)
	account.Set("phone_number_id", phoneNumberID)
	account.Set("access_token", accessToken)
	account.Set("verified_name", verifiedName)
	account.Set("status", "connected")
	account.Set("is_active", true)
	account.Set("disconnected_at", "")

	if err := app.Save(account); err != nil {
		return nil, fmt.Errorf("store the connected account: %w", err)
	}

	return account, nil
}

// AccountFor returns one person's connected Cloud API account, or
// sql.ErrNoRows when they have none.
//
// Resolving on all three of user_id, account_type and is_active is what keeps
// a signup off a retired QR account: the route this was ported from matched on
// user_id alone and overwrote one in place. The filter binds its parameters
// rather than interpolating them, because a user id reaching a filter string
// unescaped is an injection into the query language.
func AccountFor(app core.App, userID string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(
		migrations.WhatsAppAccountsCollection,
		"user_id = {:userID} && account_type = {:accountType} && is_active = true",
		dbx.Params{"userID": userID, "accountType": BusinessAPIAccount},
	)
}
