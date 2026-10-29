package whatsapp

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
)

// ExchangeTokenPath is where the Embedded Signup button posts its
// authorization code.
const ExchangeTokenPath = "/api/whatsapp/exchange-token"

// Register binds this package's routes onto r.
//
// Only concrete method/path pairs are registered: an all-method /api/...
// wildcard conflicts with the SPA's GET /{path...} catch-all, and the std
// ServeMux answers that conflict with a panic at boot rather than a precedence
// rule.
func Register(r *router.Router[*core.RequestEvent], client *meta.Client) {
	// Registering a pattern twice panics inside BuildMux for the same reason,
	// so the guard is cheap insurance against two call sites.
	if r.HasRoute(http.MethodPost, ExchangeTokenPath) {
		return
	}

	r.POST(ExchangeTokenPath, exchangeToken(client)).Bind(apis.RequireAuth("users"))
}

// exchangeToken completes the signup for the person making the request.
//
// The account is always the caller's own: the user id comes from the verified
// auth record, never from the request body, so there is nothing to forge.
func exchangeToken(client *meta.Client) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var signup Signup
		if err := e.BindBody(&signup); err != nil {
			return e.BadRequestError("The signup could not be read.", err)
		}

		account, err := Connect(e.Request.Context(), e.App, client, e.Auth.Id, signup)

		switch {
		case errors.Is(err, ErrIncompleteSignup):
			return e.BadRequestError("The signup did not include a code and a WhatsApp Business account id.", err)

		case errors.Is(err, meta.ErrPhoneNumberUnresolved):
			// Meta sent no number and the WABA does not have exactly one, so
			// there is nothing this app can do with the signup. Saying which
			// is which is the difference between a user retrying forever and
			// a user going to look at their WABA.
			return e.BadRequestError("Could not tell which WhatsApp number was connected. Check that the business account has exactly one number.", err)

		case err != nil:
			// Meta's raw response is not echoed: the failing token exchange
			// carries the app secret in its query string, and its error body
			// is written for a developer reading a log, not for a browser.
			e.App.Logger().Error("the WhatsApp signup failed", "user_id", e.Auth.Id, "error", err)

			return e.InternalServerError("Connecting the WhatsApp account failed. Please try again.", err)
		}

		// The account itself is not returned: the page reads it from the
		// collection, where the hidden fields are stripped for it.
		return e.JSON(http.StatusOK, map[string]any{
			"success":         true,
			"account_id":      account.Id,
			"phone_number_id": account.GetString("phone_number_id"),
			"verified_name":   account.GetString("verified_name"),
		})
	}
}
