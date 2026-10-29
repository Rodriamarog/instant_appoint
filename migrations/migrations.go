// Package migrations holds the PocketBase schema migrations for this app.
//
// Each migration is a separate file named <unix-timestamp>_<name>.go whose
// init() calls m.Register(up, down) from
// github.com/pocketbase/pocketbase/migrations. PocketBase orders the list by
// filename, so the timestamp prefix is what fixes the order.
//
// cmd/server blank-imports this package so those init() functions run, and
// registers the migratecmd plugin with Automigrate turned off: collections are
// defined here in code, and edits made in the admin UI are never written back
// as migration files.
//
// `serve` applies pending migrations on boot; `migrate up` applies them
// without starting the server.
package migrations
