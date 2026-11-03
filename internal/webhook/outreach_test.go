package webhook_test

import (
	"testing"

	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// The control is one comparison, tested on its own because its two failures
// are not symmetric and the asymmetry runs the other way from the mode
// switch's. A clinic that asked for silence and still gets it wrong hears
// from its patients; a clinic that never asked for silence and is given it
// loses the only check on appointments the assistant writes into its
// calendar, and finds out when somebody turns up for a booking they never
// agreed to.
func TestOnlyADeliberateOffSilencesTheAssistant(t *testing.T) {
	for _, c := range []struct {
		outreach webhook.Outreach
		want     bool
	}{
		{webhook.OutreachOn, true},
		{webhook.OutreachOff, false},

		// A row written before this column existed, read before the
		// backfill has run, reaches here empty. It is not a clinic asking
		// for anything, and the behaviour it predates is the receipt being
		// sent.
		{webhook.Outreach(""), true},

		// Neither is a value nobody here can read. The gate asks "has this
		// clinic said off", never "has this clinic said on".
		{webhook.Outreach("OFF"), true},
		{webhook.Outreach("disabled"), true},
	} {
		if got := c.outreach.MessagesPatients(); got != c.want {
			t.Errorf("Outreach(%q).MessagesPatients() = %v, want %v", c.outreach, got, c.want)
		}
	}
}

// The two answers are stored as these words. They are asserted as literals
// rather than read off the constants because the strings are a storage
// format: migrations/1793232012_patient_outreach.go writes them, Configuración
// writes them, and renaming a constant would leave every existing row
// meaning something this build no longer answers to.
func TestTheStoredSpellingOfTheOutreachSetting(t *testing.T) {
	for outreach, want := range map[webhook.Outreach]string{
		webhook.OutreachOn:  "on",
		webhook.OutreachOff: "off",
	} {
		if string(outreach) != want {
			t.Errorf("the setting is stored as %q, want %q", string(outreach), want)
		}
	}
}
