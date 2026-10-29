package whatsapp

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
)

// The template endpoints. They hold the account's Meta access token, which is
// why they are server routes rather than calls the page makes itself.
const (
	TemplatesPath      = "/api/whatsapp/templates"
	CreateTemplatePath = "/api/whatsapp/templates/create"
	SendTemplatePath   = "/api/whatsapp/templates/send"
)

// listTemplates answers with the signed-in person's own message templates.
//
// The WABA comes from their stored account. The route this was ported from
// pinned one business account id in the filter, so every tenant but that one
// got a 404 from a page that had just told them they were connected.
func listTemplates(client *meta.Client) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		account, err := requireAccount(e)
		if err != nil {
			return err
		}

		templates, err := client.ListTemplates(e.Request.Context(),
			account.GetString("waba_id"), account.GetString("access_token"))
		if err != nil {
			return metaError(e, "Could not load your templates from Meta.", err)
		}

		return e.JSON(http.StatusOK, map[string]any{"templates": templates})
	}
}

// requireAccount resolves the caller's connected account, or returns the
// response to send when they have none.
func requireAccount(e *core.RequestEvent) (*core.Record, error) {
	account, err := AccountFor(e.App, e.Auth.Id)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, e.NotFoundError("No connected WhatsApp account found. Connect a number first.", err)
	case err != nil:
		e.App.Logger().Error("could not resolve the WhatsApp account", "user_id", e.Auth.Id, "error", err)

		return nil, e.InternalServerError("Could not read your WhatsApp account.", err)
	}

	return account, nil
}

// metaError turns a Graph failure into one readable sentence.
//
// Meta's raw body reaches neither the browser nor the log: a failing call can
// carry the app secret in the query string it echoes. meta.Explain and
// meta.Sanitise own that rule; see them for what is left out and why.
func metaError(e *core.RequestEvent, fallback string, err error) error {
	e.App.Logger().Error("a Meta Graph call failed", "user_id", e.Auth.Id, "error", meta.Sanitise(err))

	if explanation, ok := meta.Explain(err); ok {
		return e.InternalServerError("Meta rejected the request: "+explanation, err)
	}

	return e.InternalServerError(fallback, err)
}

// TemplateDraft is what the create form posts. The field names are the ones
// the page already sends, kept so the port is a swap rather than a rewrite.
type templateDraftRequest struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Language string `json:"language"`
	Body     string `json:"body"`
}

// createTemplate submits a draft for Meta's approval.
//
// Nothing is stored here: templates live at Meta, and the freshly created one
// comes back PENDING and unusable until a human at Meta approves it.
func createTemplate(client *meta.Client) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var draft templateDraftRequest
		if err := e.BindBody(&draft); err != nil {
			return e.BadRequestError("The template could not be read.", err)
		}

		// The form checks these too. It is checked again here because the
		// endpoint is reachable without the form, and Meta answers a missing
		// field with a 400 that costs a round trip to learn nothing.
		if draft.Name == "" || draft.Category == "" || draft.Language == "" || draft.Body == "" {
			return e.BadRequestError("A template needs a name, a category, a language and a body.", nil)
		}

		account, err := requireAccount(e)
		if err != nil {
			return err
		}

		created, err := client.CreateTemplate(e.Request.Context(),
			account.GetString("waba_id"), account.GetString("access_token"), meta.TemplateDraft{
				Name:     draft.Name,
				Category: draft.Category,
				Language: draft.Language,
				Body:     draft.Body,
			})
		if err != nil {
			return metaError(e, "Submitting the template to Meta failed.", err)
		}

		return e.JSON(http.StatusOK, map[string]any{"success": true, "template": created})
	}
}

// templateSendRequest is what the page posts to send one template as a test.
type templateSendRequest struct {
	TemplateName string `json:"template_name"`
	LanguageCode string `json:"language_code"`

	// To is digits including the country code and without a leading +, the
	// form both the webhook and the stored customer numbers use.
	To string `json:"to"`

	// BodyParameters fill the template's {{1}}, {{2}} placeholders, in order.
	// A template with none sends an empty slice, which omits the components
	// key entirely — Meta rejects an empty array.
	BodyParameters []string `json:"body_parameters"`
}

// sendTemplate sends one approved template from the caller's own number.
//
// The sending number is the account's phone_number_id rather than anything in
// the request: a person may only send as themselves.
func sendTemplate(client *meta.Client) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var request templateSendRequest
		if err := e.BindBody(&request); err != nil {
			return e.BadRequestError("The send could not be read.", err)
		}

		if request.TemplateName == "" || request.LanguageCode == "" || request.To == "" {
			return e.BadRequestError("A test send needs a template, its language and a number to send to.", nil)
		}

		account, err := requireAccount(e)
		if err != nil {
			return err
		}

		messageID, err := client.SendTemplate(e.Request.Context(),
			account.GetString("phone_number_id"), account.GetString("access_token"),
			request.To, request.TemplateName, request.LanguageCode, request.BodyParameters)
		if err != nil {
			return metaError(e, "Sending the template failed.", err)
		}

		return e.JSON(http.StatusOK, map[string]any{"success": true, "message_id": messageID})
	}
}
