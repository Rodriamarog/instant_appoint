package pbstore_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
)

// mux builds the real request multiplexer from a registered router, which is
// where a conflicting pattern pair turns into a panic.
func mux(t *testing.T, register func(*router.Router[*core.RequestEvent])) http.Handler {
	t.Helper()

	r := router.NewRouter(func(w http.ResponseWriter, req *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		event := new(core.RequestEvent)
		event.Response, event.Request = w, req

		return event, nil
	})
	register(r)

	built, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	return built
}

// /api/webhook is a fixed external contract: the Neurocrow router already
// posts there, and it cannot be told to post somewhere else without a
// coordinated change on both sides. /webhook is the second path the old app
// exposed, kept until the router is confirmed to use only the first.
func TestRegisterWebhookAnswersBothPaths(t *testing.T) {
	handler := mux(t, func(r *router.Router[*core.RequestEvent]) {
		pbstore.RegisterWebhook(r, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write(body)
		}))
	})

	for _, path := range []string{"/api/webhook", "/webhook"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"entry":[]}`)))

			if response.Code != http.StatusTeapot {
				t.Errorf("status = %d, want the wrapped handler's %d", response.Code, http.StatusTeapot)
			}
			// The handler must see the body, which is the thing the signature
			// is computed over.
			if got := response.Body.String(); got != `{"entry":[]}` {
				t.Errorf("the handler read %q, want the request body", got)
			}
		})
	}
}

// Registering a pattern twice panics inside BuildMux at boot rather than
// overriding, so two call sites would take the whole server down.
func TestRegisterWebhookIsSafeToCallTwice(t *testing.T) {
	mux(t, func(r *router.Router[*core.RequestEvent]) {
		pbstore.RegisterWebhook(r, http.NotFoundHandler())
		pbstore.RegisterWebhook(r, http.NotFoundHandler())
	})
}
