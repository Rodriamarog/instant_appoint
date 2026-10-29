package web_test

import (
	"io/fs"
	"testing"

	"github.com/Rodriamarog/instant_appoint/web"
)

// The server serves Dist() directly and falls back to index.html for every
// client-side route, so Dist() must always contain one — including in a fresh
// clone where Vite has never run and only the placeholder is embedded.
func TestDistAlwaysContainsAnIndex(t *testing.T) {
	body, err := fs.ReadFile(web.Dist(), "index.html")
	if err != nil {
		t.Fatalf("reading index.html from Dist(): %v", err)
	}
	if len(body) == 0 {
		t.Fatal("index.html is empty")
	}
}
