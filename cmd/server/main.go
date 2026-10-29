// Command server is the whole application: PocketBase embedded as a library,
// serving the React SPA that is compiled into this binary.
//
// It is wiring only. Behaviour lives in internal/* and migrations/.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	// A pure-Go binary has no system tzdata to fall back on, so embed the
	// database; reminder scheduling resolves named zones such as
	// America/Tijuana at runtime.
	_ "time/tzdata"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/Rodriamarog/instant_appoint/internal/gemini"
	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
	"github.com/Rodriamarog/instant_appoint/internal/reminderjob"
	"github.com/Rodriamarog/instant_appoint/internal/seed"
	"github.com/Rodriamarog/instant_appoint/internal/server"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
	"github.com/Rodriamarog/instant_appoint/internal/whatsapp"
	_ "github.com/Rodriamarog/instant_appoint/migrations"
	"github.com/Rodriamarog/instant_appoint/web"
)

func main() {
	app := pocketbase.New()

	// Automigrate would write a migration file for every collection edit made
	// in the admin UI, which fights hand-written Go migrations. Off means the
	// code in migrations/ is the only source of schema truth.
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangGo,
		Automigrate:  false,
		Dir:          "migrations",
	})

	// Email verification needs mail credentials that only the project owner
	// has, so a fresh database has no account anyone can log in with. This
	// writes one, already verified.
	app.RootCmd.AddCommand(seed.Command(app))

	dist := web.Dist()

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if !web.IsBuilt() {
			se.App.Logger().Warn("no web/dist bundle was embedded; serving the placeholder page. Run `npm run build` in web/ and rebuild.")
		}

		// The Embedded Signup exchange spends the Meta app secret, so it runs
		// on the server rather than in the browser. Without the credentials
		// the rest of the app still works; only connecting a new number
		// fails, and a warning here is easier to act on than the 500 it would
		// otherwise produce weeks later.
		appID, appSecret := os.Getenv("META_APP_ID"), os.Getenv("META_APP_SECRET")
		if appID == "" || appSecret == "" {
			se.App.Logger().Warn("META_APP_ID or META_APP_SECRET is unset; connecting a WhatsApp number will fail until both are.")
		}
		client := meta.New(appID, appSecret)

		// Every Graph call this binary makes is a real request to Meta, which
		// makes the template and signup paths unexercisable without a live
		// business account. Pointing the host elsewhere is the one seam that
		// lets an automated check drive them end to end; it is loud because
		// an environment that sets it by accident is talking to something
		// that is not Meta.
		if base := os.Getenv("META_GRAPH_BASE_URL"); base != "" {
			se.App.Logger().Warn("META_GRAPH_BASE_URL is set: Graph calls go to a stand-in, not to Meta.", "base_url", base)
			client.BaseURL = base
		}

		whatsapp.Register(se.Router, client)

		pbstore.RegisterWebhook(se.Router, inboundWebhook(se.App, client))

		// Appointment reminders. PocketBase starts its cron from an OnServe
		// hook, so this is scheduled only under `serve` — never under
		// `migrate` or `seed`, which would otherwise start sending.
		reminderjob.Register(se.App, client)

		// Route patterns are matched by specificity, not registration
		// order, so later tickets can add their own /api routes from their
		// own OnServe handlers regardless of where this one sits.
		server.Register(se.Router, dist)

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// inboundWebhook assembles the handler that turns a WhatsApp delivery into a
// stored message and an AI reply.
//
// The handler itself knows nothing about PocketBase, Gemini or Meta — it is
// written against a Store interface and two function fields, which is what
// lets its whole behaviour be tested with no database and no network. This is
// where the real implementations are substituted for the fakes.
func inboundWebhook(app core.App, client *meta.Client) http.Handler {
	// Both secrets are read here rather than at package load, so a deployment
	// that sets them in the process environment after the binary is built
	// still picks them up, and so a missing one is reported once at boot
	// instead of as a puzzle in a log weeks later.
	secret := os.Getenv("NEUROCROW_ROUTER_SECRET")
	if secret == "" {
		// An empty secret rejects every delivery rather than accepting every
		// delivery, so this is a silent outage unless it is said out loud.
		app.Logger().Warn("NEUROCROW_ROUTER_SECRET is unset; every webhook delivery will be rejected and no customer message will arrive.")
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		app.Logger().Warn("GEMINI_API_KEY is unset; messages will still be stored but no automatic reply will be sent.")
	}
	ai := gemini.New(geminiKey)

	// The same seam META_GRAPH_BASE_URL opens for Meta, for the same reason:
	// without it a check of the inbound path can prove that a message is
	// stored but not that a reply is written and sent, which is half the
	// behaviour. Loud, because an environment that sets it by accident is
	// sending customer messages to something that is not Google.
	if base := os.Getenv("GEMINI_BASE_URL"); base != "" {
		app.Logger().Warn("GEMINI_BASE_URL is set: replies are written by a stand-in, not by Gemini.", "base_url", base)
		ai.BaseURL = base
	}

	return webhook.New(webhook.Deps{
		Secret: secret,
		Now:    time.Now,
		Store:  pbstore.NewWebhookStore(app),
		GenerateReply: func(ctx context.Context, history []webhook.ConversationMessage, text string) (string, error) {
			// The two packages spell the roles the same way and neither
			// imports the other, which is the point: the handler does not
			// depend on which model answers.
			turns := make([]gemini.Message, 0, len(history))
			for _, turn := range history {
				turns = append(turns, gemini.Message{Role: turn.Role, Content: turn.Content})
			}

			return ai.GenerateReply(ctx, turns, text)
		},
		SendMessage: client.SendText,
		Logger:      app.Logger(),
	})
}
