package seed_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Rodriamarog/instant_appoint/internal/seed"
)

// Nothing in this repo can connect a real WhatsApp number without a human
// clicking through Meta's signup, so every ticket that needs a connected
// account needs this fixture to stand in for one.
func TestApplyCreatesAConnectedWhatsAppAccount(t *testing.T) {
	app := newApp(t)

	result, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if result.WhatsAppAccountID == "" {
		t.Fatal("no WhatsApp account id was reported")
	}

	account, err := app.FindRecordById("whatsapp_accounts", result.WhatsAppAccountID)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}

	for field, want := range map[string]string{
		"user_id":         result.UserID,
		"account_type":    "business_api",
		"status":          "connected",
		"phone_number_id": result.WhatsAppPhoneNumberID,
		"waba_id":         result.WhatsAppWABAID,
		"verified_name":   result.WhatsAppVerifiedName,
	} {
		if got := account.GetString(field); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if !account.GetBool("is_active") {
		t.Error("the seeded account is not active, so no handler will find it")
	}

	// A token is needed for the account to exercise the code paths that read
	// one, and it must be obviously fake so nobody tries it against Meta.
	token := account.GetString("access_token")
	if token == "" {
		t.Error("the seeded account has no access token")
	}
	if !strings.Contains(strings.ToUpper(token), "FAKE") {
		t.Errorf("access_token = %q; a seeded token must announce that it is not real", token)
	}
}

func TestSeededAccountStillHidesItsToken(t *testing.T) {
	app := newApp(t)

	result, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	account, err := app.FindRecordById("whatsapp_accounts", result.WhatsAppAccountID)
	if err != nil {
		t.Fatalf("FindRecordById: %v", err)
	}

	encoded, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), "access_token") {
		t.Errorf("the seeded account serializes its token: %s", encoded)
	}
}

// Seeding runs repeatedly against a database that may already be seeded, and a
// second connected account for the same person would make every "the user's
// account" lookup ambiguous.
func TestApplyDoesNotDuplicateTheWhatsAppAccount(t *testing.T) {
	app := newApp(t)

	first, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	second, err := seed.Apply(app)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	if first.WhatsAppAccountID != second.WhatsAppAccountID {
		t.Errorf("account id changed between runs: %q then %q", first.WhatsAppAccountID, second.WhatsAppAccountID)
	}

	accounts, err := app.FindAllRecords("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindAllRecords: %v", err)
	}
	if len(accounts) != 1 {
		t.Errorf("after two runs there are %d accounts, want 1", len(accounts))
	}
}
