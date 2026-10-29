package pbstore

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// WebhookPaths are where the Neurocrow router delivers.
//
// /api/webhook is a fixed external contract: the router already posts there,
// and changing it means a coordinated change on a system this repository does
// not contain. /webhook is the second path the old app exposed; it stays until
// the router is confirmed to use only the first, because a delivery to a path
// that 404s is a customer message on the floor.
var WebhookPaths = []string{"/api/webhook", "/webhook"}

// RegisterWebhook mounts the webhook handler on both delivery paths.
//
// One handler, mounted twice, not two handlers: the handler holds the
// in-flight guard that stops two simultaneous deliveries of one message both
// being answered, and two instances would not see each other's work.
//
// The paths are concrete methods on concrete paths on purpose. An all-method
// "/api/..." wildcard conflicts with the SPA's "GET /{path...}" catch-all —
// neither pattern is more specific than the other — and the standard
// ServeMux answers that conflict by panicking at boot.
func RegisterWebhook(r *router.Router[*core.RequestEvent], handler http.Handler) {
	// Registering a pattern twice panics inside BuildMux for the same reason,
	// so the guard is cheap insurance against two call sites.
	wrapped := apis.WrapStdHandler(handler)

	for _, path := range WebhookPaths {
		if r.HasRoute(http.MethodPost, path) {
			continue
		}
		r.POST(path, wrapped)
	}
}
