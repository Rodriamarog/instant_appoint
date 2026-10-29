package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// newApp returns a bootstrapped app with every migration applied, backed by a
// throwaway data directory.
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

// Every field here is written or read by code somewhere. A field the schema
// does not declare makes the write that carries it fail with a PocketBase
// "unknown field" validation error, which for verified_name takes the whole
// token exchange down with it.
func TestWhatsAppAccountsDeclaresEveryFieldTheCodeUses(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	want := []string{
		"user_id", "session_id", "status", "qr_code", "phone_number",
		"is_active", "disconnect_reason", "account_type", "waba_id",
		"phone_number_id", "verified_name", "access_token", "disconnected_at",
	}
	for _, name := range want {
		if collection.Fields.GetByName(name) == nil {
			t.Errorf("field %q is missing", name)
		}
	}
}

// The token is a long-lived Meta credential on a collection the browser lists
// directly. Hidden keeps it out of REST and realtime payloads while Go still
// reads it with record.GetString.
func TestWhatsAppAccountsHidesTheSecretFields(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	for _, name := range []string{"access_token", "qr_code", "session_id"} {
		field := collection.Fields.GetByName(name)
		if field == nil {
			t.Errorf("field %q is missing", name)
			continue
		}
		if !field.GetHidden() {
			t.Errorf("field %q is not hidden; it would ship to the browser", name)
		}
	}
}

// Rules are the only protection left once the SPA talks to PocketBase
// directly. Reads are owner-scoped; writes happen in Go, which bypasses rules,
// so no client ever needs them.
func TestWhatsAppAccountsRulesAreOwnerReadServerWrite(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	owner := "user_id = @request.auth.id"
	for _, tc := range []struct {
		name string
		rule *string
	}{
		{"list", collection.ListRule},
		{"view", collection.ViewRule},
	} {
		if tc.rule == nil || *tc.rule != owner {
			t.Errorf("%s rule = %v, want %q", tc.name, tc.rule, owner)
		}
	}

	for _, tc := range []struct {
		name string
		rule *string
	}{
		{"create", collection.CreateRule},
		{"update", collection.UpdateRule},
		{"delete", collection.DeleteRule},
	} {
		if tc.rule != nil {
			t.Errorf("%s rule = %q, want nil (superuser and server-side Go only)", tc.name, *tc.rule)
		}
	}
}

// is_active must not be Required: PocketBase reads Required on a bool as "must
// be true", which would make it impossible to store a disconnected account.
func TestWhatsAppAccountsCanStoreADisconnectedAccount(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", "someuser0000000")
	record.Set("session_id", "business_someuser0000000")
	record.Set("status", "disconnected")
	record.Set("is_active", false)
	record.Set("disconnected_at", "2026-01-02T03:04:05.000Z")

	if err := app.Save(record); err != nil {
		t.Fatalf("Save a disconnected account: %v", err)
	}
}
