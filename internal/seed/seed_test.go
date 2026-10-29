package seed_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/seed"
	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// newApp returns a bootstrapped app with every migration applied, backed by a
// throwaway data directory. The blank import above is what puts this app's own
// migrations (and so the users.business_name field) into the list.
func newApp(t *testing.T) core.App {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		if err := app.ResetBootstrapState(); err != nil {
			t.Errorf("ResetBootstrapState: %v", err)
		}
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	return app
}

func TestApplyCreatesAnAlreadyVerifiedAccount(t *testing.T) {
	app := newApp(t)

	result, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	user, err := app.FindAuthRecordByEmail("users", result.Email)
	if err != nil {
		t.Fatalf("FindAuthRecordByEmail(%q): %v", result.Email, err)
	}

	// Email verification needs SMTP credentials nobody has in development, and
	// the login page refuses an unverified account. Without this the seeded
	// account cannot be used at all.
	if !user.Verified() {
		t.Error("seeded account is not verified")
	}
	if !user.ValidatePassword(result.Password) {
		t.Error("the printed password does not authenticate the seeded account")
	}
	if got := user.GetString("business_name"); got == "" {
		t.Error("business_name is empty; the reminder copy falls back to a placeholder")
	}
	if result.UserID != user.Id {
		t.Errorf("result.UserID = %q, want %q", result.UserID, user.Id)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	app := newApp(t)

	first, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// A second run is the normal case: the command is re-run on every
	// environment rebuild, and the email is unique, so a blind create would
	// fail validation here.
	second, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	if first.UserID != second.UserID {
		t.Errorf("user id changed between runs: %q then %q", first.UserID, second.UserID)
	}

	users, err := app.FindAllRecords("users")
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("after two runs there are %d users, want 1", len(users))
	}
}

func TestApplyRepairsADriftedAccount(t *testing.T) {
	app := newApp(t)

	if _, err := seed.Apply(app); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Anything can happen to the account between runs — a password changed in
	// the admin UI, verification toggled off. Re-running is the documented way
	// back in, so it has to converge, not just skip an existing row.
	user, err := app.FindAuthRecordByEmail("users", seed.TestEmail)
	if err != nil {
		t.Fatalf("FindAuthRecordByEmail: %v", err)
	}
	user.SetPassword("something-else-entirely")
	user.SetVerified(false)
	if err := app.Save(user); err != nil {
		t.Fatalf("Save drifted user: %v", err)
	}

	result, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	user, err = app.FindAuthRecordByEmail("users", seed.TestEmail)
	if err != nil {
		t.Fatalf("FindAuthRecordByEmail: %v", err)
	}
	if !user.Verified() {
		t.Error("verification was not restored")
	}
	if !user.ValidatePassword(result.Password) {
		t.Error("the printed password was not restored")
	}
}

func TestCommandPrintsCredentialsAScriptCanRead(t *testing.T) {
	app := newApp(t)

	cmd := seed.Command(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Later tickets log in as this account from scripts, so the output has to
	// be machine-readable without a parser: one KEY=value per line.
	printed := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			printed[key] = value
		}
	}

	if printed["SEED_EMAIL"] != seed.TestEmail {
		t.Errorf("SEED_EMAIL = %q, want %q (output: %q)", printed["SEED_EMAIL"], seed.TestEmail, out.String())
	}
	if printed["SEED_PASSWORD"] != seed.TestPassword {
		t.Errorf("SEED_PASSWORD = %q, want %q (output: %q)", printed["SEED_PASSWORD"], seed.TestPassword, out.String())
	}

	user, err := app.FindAuthRecordByEmail("users", printed["SEED_EMAIL"])
	if err != nil {
		t.Fatalf("the printed email does not name a real account: %v", err)
	}
	if printed["SEED_USER_ID"] != user.Id {
		t.Errorf("SEED_USER_ID = %q, want %q", printed["SEED_USER_ID"], user.Id)
	}
	if !user.ValidatePassword(printed["SEED_PASSWORD"]) {
		t.Error("the printed password does not authenticate the account")
	}
}
