// Package web embeds the built single-page application so that the server
// binary is self-contained: once compiled, nothing is read from disk.
package web

import (
	"embed"
	"io/fs"
)

// The "all:" prefix is required — Vite emits asset directories whose names can
// start with "_", and a plain //go:embed silently skips those.
//
// web/dist is gitignored apart from a committed .gitkeep, so this pattern
// always matches at least one file and a fresh clone compiles without having
// run Vite first.
//
//go:embed all:dist
var distFS embed.FS

// placeholder is served when dist holds no index.html, i.e. when the binary was
// compiled before `npm run build` was ever run in web/.
//
//go:embed placeholder
var placeholderFS embed.FS

// Dist returns the file system holding the built SPA, rooted at the directory
// that contains index.html.
//
// When no build output was embedded it returns a one-page placeholder file
// system instead, so the binary still boots and says why the UI is missing.
func Dist() fs.FS {
	if sub, err := fs.Sub(distFS, "dist"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			return sub
		}
	}

	sub, err := fs.Sub(placeholderFS, "placeholder")
	if err != nil {
		// Unreachable: the placeholder directory is embedded unconditionally.
		panic(err)
	}

	return sub
}

// IsBuilt reports whether a real Vite build was embedded into this binary.
func IsBuilt() bool {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return false
	}
	_, err = fs.Stat(sub, "index.html")
	return err == nil
}
