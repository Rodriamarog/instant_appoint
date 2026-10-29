package arch_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// forbidden is what the packages below may not depend on.
//
// PocketBase is not a library you can hold at arm's length: importing it pulls
// in a database, a migration runner and a server. A package that imports it
// can only be tested by standing all of that up, and a test that needs a
// database gets written once and then avoided.
const forbidden = "github.com/pocketbase/"

// pure names the packages that must stay free of it, by directory under
// internal/.
//
// These are the two pieces of this app whose behaviour is a long list of cases
// the outside world can produce — every shape Meta can deliver, every way a
// reminder can fall due — and the only way that list stays honest is if adding
// a case to it costs one Go test and no setup. Both are written against
// interfaces; internal/pbstore is where the implementations live.
//
// reminders belongs to ticket #9 and may not exist yet. It is listed now so
// that it is covered the moment it does, rather than after someone notices.
var pure = []string{"webhook", "routersig", "reminders"}

// The rule holds for test files too. A test that reaches for PocketBase to set
// up a fixture is how the dependency comes back: the package still compiles
// without it, but running its tests needs a database again.
func TestThePurePackagesDoNotImportTheDatabaseFramework(t *testing.T) {
	for _, name := range pure {
		t.Run(name, func(t *testing.T) {
			directory := filepath.Join("..", name)
			if _, err := os.Stat(directory); os.IsNotExist(err) {
				t.Skipf("internal/%s does not exist yet", name)
			}

			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatalf("read internal/%s: %v", name, err)
			}

			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}

				path := filepath.Join(directory, entry.Name())
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					t.Fatalf("parse %s: %v", path, err)
				}

				for _, spec := range file.Imports {
					imported, err := strconv.Unquote(spec.Path.Value)
					if err != nil {
						continue
					}
					if strings.HasPrefix(imported, forbidden) {
						t.Errorf("%s imports %s; internal/%s must be testable with no database", path, imported, name)
					}
				}
			}
		})
	}
}

// Parsing the files above catches the import somebody types. This catches the
// one they acquire: a helper added to a package that itself imports
// PocketBase, which compiles in and drags the database along without the word
// appearing anywhere in internal/webhook.
func TestThePurePackagesDoNotDependOnTheDatabaseFrameworkIndirectly(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH to resolve the dependency graph with")
	}

	for _, name := range pure {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join("..", name)); os.IsNotExist(err) {
				t.Skipf("internal/%s does not exist yet", name)
			}

			// -test includes the dependencies of the package's own tests, so a
			// fixture that reaches for PocketBase is caught too.
			output, err := exec.Command("go", "list", "-deps", "-test", "./../"+name).CombinedOutput()
			if err != nil {
				t.Fatalf("go list -deps ./../%s: %v\n%s", name, err, output)
			}

			for _, line := range strings.Split(string(output), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), forbidden) {
					t.Errorf("internal/%s depends on %s through another package; it must be testable with no database",
						name, strings.TrimSpace(line))
				}
			}
		})
	}
}
