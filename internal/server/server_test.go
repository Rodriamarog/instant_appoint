package server_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Rodriamarog/instant_appoint/internal/server"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

const shell = `<!doctype html><html><body>SPA SHELL</body></html>`

func testDist() fs.FS {
	return fstest.MapFS{
		"index.html":    {Data: []byte(shell)},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
}

// newMux builds the same mux the real server builds, minus PocketBase's own
// routes, so the application's HTTP surface can be exercised without a
// database or a listener.
func newMux(t *testing.T, dist fs.FS) http.Handler {
	t.Helper()

	r := router.NewRouter(func(w http.ResponseWriter, req *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		e := new(core.RequestEvent)
		e.Response = w
		e.Request = req
		return e, nil
	})

	server.Register(r, dist)

	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	return mux
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestUnknownAPIPathReturnsAJSON404(t *testing.T) {
	mux := newMux(t, testDist())

	// Without this a health probe against a mistyped endpoint reads the HTML
	// shell with a 200 and reports the service as healthy forever.
	for _, path := range []string{"/api/does-not-exist", "/api/whatsapp/nope", "/api/"} {
		rec := get(t, mux, path)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("GET %s: Content-Type = %q, want application/json", path, ct)
		}

		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("GET %s: body %q is not JSON: %v", path, rec.Body.String(), err)
			continue
		}
		if status, _ := body["status"].(float64); status != http.StatusNotFound {
			t.Errorf("GET %s: body status = %v, want 404", path, body["status"])
		}
	}
}

func TestRegisteringTwiceDoesNotPanic(t *testing.T) {
	// A duplicate pattern is a std ServeMux conflict, which surfaces as a
	// panic at startup rather than an override or an error.
	r := router.NewRouter(func(w http.ResponseWriter, req *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		e := new(core.RequestEvent)
		e.Response = w
		e.Request = req
		return e, nil
	})

	server.Register(r, testDist())
	server.Register(r, testDist())

	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	rec := get(t, mux, "/inicio")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestUnknownAPIPathReturnsAJSON404ForOtherMethods(t *testing.T) {
	mux := newMux(t, testDist())

	// The SPA catch-all is GET-only, so non-GET misses fall through to the
	// router's own all-method handler. Pinned here because the GET-qualified
	// /api 404 above relies on it.
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/does-not-exist", nil))

		if rec.Code != http.StatusNotFound {
			t.Errorf("%s /api/does-not-exist: status = %d, want 404", method, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s /api/does-not-exist: Content-Type = %q, want application/json", method, ct)
		}
	}
}

func TestMissingAssetIsA404AndNotTheShell(t *testing.T) {
	mux := newMux(t, testDist())

	if rec := get(t, mux, "/assets/app.js"); rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js: status = %d, want 200 (sanity check)", rec.Code)
	}

	// Answering a missing module with the HTML shell and a 200 makes the
	// browser fail with an opaque MIME-type error instead of a plain 404.
	rec := get(t, mux, "/assets/missing.js")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "SPA SHELL") {
		t.Errorf("body = %q, want a 404 rather than the app shell", body)
	}
	if ct := rec.Header().Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want anything but HTML", ct)
	}
}

func TestClientSideRoutesServeTheAppShell(t *testing.T) {
	mux := newMux(t, testDist())

	// A deep link and a refresh on it are the same GET as far as the server is
	// concerned: no file exists at any of these paths, yet all must render
	// the app. /chat is the one #21 deleted: the client-side 404 can only
	// tell somebody their bookmark is stale if the shell reaches them first.
	for _, path := range []string{"/inicio", "/configuracion/plantillas", "/chat"} {
		rec := get(t, mux, path)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", path, rec.Code)
		}
		if body := rec.Body.String(); body != shell {
			t.Errorf("GET %s: body = %q, want the app shell", path, body)
		}
	}
}

func TestRootServesTheAppShell(t *testing.T) {
	rec := get(t, newMux(t, testDist()), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != shell {
		t.Fatalf("body = %q, want the app shell", body)
	}
}

func TestTheAppShellLetsAPopupKeepItsOpener(t *testing.T) {
	mux := newMux(t, testDist())

	// Meta's Embedded Signup reports its result by posting a message to
	// window.opener. PocketBase's default Cross-Origin-Opener-Policy of
	// same-origin puts a cross-origin popup in its own browsing context group
	// and severs that link both ways, so the result can never arrive and
	// connecting a number fails without a sign. The shell relaxes it to
	// same-origin-allow-popups, which keeps the half that matters: no other
	// window can get a reference into this one.
	for _, path := range []string{"/", "/configuracion/whatsapp", "/inicio"} {
		rec := get(t, mux, path)

		if got := rec.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin-allow-popups" {
			t.Errorf("GET %s: Cross-Origin-Opener-Policy = %q, want same-origin-allow-popups", path, got)
		}
	}
}

func TestOnlyTheShellRelaxesTheOpenerPolicy(t *testing.T) {
	mux := newMux(t, testDist())

	// The relaxation exists for the one document that opens Meta's popup.
	// Everything else must be left alone so PocketBase's stricter default
	// stands.
	//
	// This harness holds no PocketBase middleware, so "untouched" is all it
	// can see: absence here means this handler set nothing, not that the
	// default is absent in a running server. What the default actually is,
	// and that the shell's override survives it, is a property of the real
	// stack — see the note on the ticket.
	for _, path := range []string{"/assets/app.js", "/assets/missing.js", "/api/does-not-exist"} {
		rec := get(t, mux, path)

		if got := rec.Header().Get("Cross-Origin-Opener-Policy"); got != "" {
			t.Errorf("GET %s: Cross-Origin-Opener-Policy = %q, want none so the default stands", path, got)
		}
	}
}
