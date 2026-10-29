// Command server is the whole application: PocketBase embedded as a library,
// serving the React SPA that is compiled into this binary.
//
// It is wiring only. Behaviour lives in internal/* and migrations/.
package main

import (
	"log"
	"os"

	// A pure-Go binary has no system tzdata to fall back on, so embed the
	// database; reminder scheduling resolves named zones such as
	// America/Tijuana at runtime.
	_ "time/tzdata"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/reminderjob"
	"github.com/Rodriamarog/instant_appoint/internal/seed"
	"github.com/Rodriamarog/instant_appoint/internal/server"
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
