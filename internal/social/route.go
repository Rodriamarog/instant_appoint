package social

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
)

// The routes. Both act on the signed-in person's own clinic: the user id
// comes from the verified auth record, never from the body.
const (
	ConnectPath    = "/api/social/connect"
	DisconnectPath = "/api/social/disconnect"
)

// Register binds the routes onto r.
func Register(r *router.Router[*core.RequestEvent], connector *Connector) {
	if r.HasRoute(http.MethodPost, ConnectPath) {
		return
	}

	r.POST(ConnectPath, connect(connector)).Bind(apis.RequireAuth("users"))
	r.POST(DisconnectPath, disconnect(connector)).Bind(apis.RequireAuth("users"))
}

// connect takes the login's code or short-lived token, or — when the login granted several
// Pages — the one the person chose from them.
func connect(connector *Connector) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var body struct {
			Code        string `json:"code"`
			AccessToken string `json:"access_token"`
			PageID      string `json:"page_id"`
		}
		if err := e.BindBody(&body); err != nil {
			return e.BadRequestError("No se pudo leer la solicitud.", err)
		}

		var connected *Connected
		var choices []PageChoice
		var err error

		switch {
		case body.Code != "":
			connected, choices, err = connector.Start(e.Request.Context(), e.Auth.Id, body.Code)
		case body.AccessToken != "":
			connected, choices, err = connector.StartWithToken(e.Request.Context(), e.Auth.Id, body.AccessToken)
		case body.PageID != "":
			connected, err = connector.Choose(e.Request.Context(), e.Auth.Id, body.PageID)
		default:
			return e.BadRequestError("Falta el código de Facebook o la página elegida.", nil)
		}

		switch {
		case errors.Is(err, ErrNoPages):
			return e.BadRequestError(
				"No elegiste ninguna página de Facebook. Vuelve a conectar y marca la página de tu negocio.", err)

		case errors.Is(err, ErrChoiceExpired), errors.Is(err, ErrUnknownPage):
			return e.BadRequestError("La lista de páginas caducó. Vuelve a conectar con Facebook.", err)

		case errors.Is(err, ErrTaken):
			return e.BadRequestError("Esa página o cuenta de Instagram ya está conectada a otra cuenta de InstantAppoint.", err)

		case err != nil:
			// Meta's raw body is never echoed: the token exchanges carry the
			// app secret in their query string. Meta's own sentence is
			// fine, and usually says what is wrong (a missing permission).
			e.App.Logger().Error("connecting a Facebook Page failed", "user_id", e.Auth.Id, "error", meta.Sanitise(err))

			message := "No se pudo conectar con Facebook. Inténtalo de nuevo."
			if explained, ok := meta.Explain(err); ok {
				message += " Facebook dijo: " + explained
			}

			return e.InternalServerError(message, nil)
		}

		if choices != nil {
			return e.JSON(http.StatusOK, map[string]any{"pages": choices})
		}

		return e.JSON(http.StatusOK, map[string]any{"connected": connected})
	}
}

func disconnect(connector *Connector) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var body struct {
			Channel string `json:"channel"`
		}
		if err := e.BindBody(&body); err != nil {
			return e.BadRequestError("No se pudo leer la solicitud.", err)
		}
		if body.Channel != Messenger && body.Channel != Instagram {
			return e.BadRequestError("Canal desconocido.", nil)
		}

		if err := connector.Disconnect(e.Request.Context(), e.Auth.Id, body.Channel); err != nil {
			return e.InternalServerError("No se pudo desconectar. Inténtalo de nuevo.", err)
		}

		return e.JSON(http.StatusOK, map[string]any{"success": true})
	}
}
