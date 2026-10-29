// Command server is the whole application: PocketBase embedded as a library,
// serving the React SPA that is compiled into this binary.
//
// It is wiring only. Behaviour lives in internal/* and migrations/.
package main

import (
	"log"

	// A pure-Go binary has no system tzdata to fall back on, so embed the
	// database; reminder scheduling resolves named zones such as
	// America/Tijuana at runtime.
	_ "time/tzdata"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/Rodriamarog/instant_appoint/internal/seed"
	"github.com/Rodriamarog/instant_appoint/internal/server"
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
