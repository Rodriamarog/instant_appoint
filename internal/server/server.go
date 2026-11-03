// Package server wires the application's own HTTP routes onto a PocketBase
// router. It owns everything PocketBase itself does not: the embedded SPA and
// the JSON 404 for unknown API paths.
package server

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// Register binds the application's routes onto r.
//
// dist is the built SPA, rooted at the directory holding index.html.
func Register(r *router.Router[*core.RequestEvent], dist fs.FS) {
	// Unknown /api/* paths must not reach the SPA catch-all below, or a
	// mistyped endpoint answers with the HTML shell and a 200. PocketBase's own
	// /api routes are more specific patterns, so they still win.
	//
	// This must stay GET-qualified. An all-method "/api/{path...}" conflicts
	// with "GET /{path...}" — neither pattern is more specific than the other,
	// and std ServeMux rejects that pair with a startup panic. Only GET needs
	// covering anyway: the SPA catch-all is GET-only, so every other method
	// already falls through to the router's own all-method JSON 404.
	if !r.HasRoute(http.MethodGet, "/api/{path...}") {
		r.GET("/api/{path...}", func(e *core.RequestEvent) error {
			return e.NotFoundError("", nil)
		})
	}

	// Registering a pattern twice panics inside BuildMux at startup — the std
	// ServeMux treats it as a conflict rather than an override — so guard the
	// catch-all even though nothing in PocketBase registers it today.
	if !r.HasRoute(http.MethodGet, "/{path...}") {
		r.GET("/{path...}", spa(dist))
	}
}

// relaxOpenerPolicy lets a popup the shell opens keep its opener, overriding
// the same-origin Cross-Origin-Opener-Policy that PocketBase's default
// security headers set on every route.
//
// same-origin puts a cross-origin popup in a browsing context group of its own
// and severs window.opener in both directions. Meta's Embedded Signup reports
// the account it connected by posting a message to its opener, so under the
// default that result can never arrive: the popup opens, the Facebook SDK is
// left holding a handle it cannot track, and connecting a number fails without
// a sign anywhere.
//
// same-origin-allow-popups keeps the half that protects this app — no other
// window can get a reference into this one — while letting a popup this page
// opened keep its opener. The policy is per-document, so only the shell needs
// it and every other response keeps the stricter default.
func relaxOpenerPolicy(e *core.RequestEvent) {
	e.Response.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
}

// spa serves dist, falling back to index.html when the request names no file.
func spa(dist fs.FS) func(*core.RequestEvent) error {
	// indexFallback is deliberately off: apis.Static's own fallback is
	// all-or-nothing and would answer every miss — including a missing
	// /assets/*.js — with the HTML shell and a 200.
	static := apis.Static(dist, false)

	return func(e *core.RequestEvent) error {
		name := path.Clean("/" + strings.TrimPrefix(e.Request.PathValue(apis.StaticWildcardParam), "/"))

		// The bare path is the shell as well, and Static answers it from
		// index.html without ever reaching the fallback below.
		if name == "/" || name == "/"+router.IndexPage {
			relaxOpenerPolicy(e)
		}

		err := static(e)
		if err == nil || !errors.Is(err, router.ErrFileNotFound) {
			return err
		}

		// Only extensionless paths can be client-side routes. Anything that
		// names a file keeps its 404 so a stale or mistyped asset URL fails
		// loudly instead of being answered with HTML.
		if path.Ext(name) != "" {
			return err
		}

		relaxOpenerPolicy(e)

		return e.FileFS(dist, router.IndexPage)
	}
}
