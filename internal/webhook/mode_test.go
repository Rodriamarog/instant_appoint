package webhook_test

import (
	"testing"

	"github.com/Rodriamarog/instant_appoint/internal/webhook"
)

// The reply decision is one comparison, and it is tested on its own because
// the cost of getting it wrong is not symmetric: a clinic whose assistant
// stays quiet notices and complains, while a clinic whose assistant talks to
// patients it was never switched on for finds out from the patients.
func TestOnlyAutopilotAnswersTheCustomer(t *testing.T) {
	for _, c := range []struct {
		mode webhook.Mode
		want bool
	}{
		{webhook.ModeAutopilot, true},
		{webhook.ModeObserver, false},
		{webhook.ModeOff, false},

		// A clinic that has never opened Configuración has no settings row at
		// all, so the mode reaches here as the zero value.
		{webhook.Mode(""), false},

		// A row written by a deploy that knows a position this build does not
		// must not be read as permission. The gate asks "is this autopilot",
		// never "is this not off".
		{webhook.Mode("supervised"), false},
		{webhook.Mode("AUTOPILOT"), false},
	} {
		if got := c.mode.RepliesAutomatically(); got != c.want {
			t.Errorf("Mode(%q).RepliesAutomatically() = %v, want %v", c.mode, got, c.want)
		}
	}
}

// The three positions are stored as these words. They are asserted as
// literals rather than read off the constants because the strings are a
// storage format: migrations/1793232008_ai_mode.go writes them, the settings
// dialog writes them, and renaming a constant would leave every existing row
// meaning something this build no longer answers to.
func TestTheStoredSpellingOfEachPosition(t *testing.T) {
	for mode, want := range map[webhook.Mode]string{
		webhook.ModeOff:       "off",
		webhook.ModeObserver:  "observer",
		webhook.ModeAutopilot: "autopilot",
	} {
		if string(mode) != want {
			t.Errorf("mode is stored as %q, want %q", string(mode), want)
		}
	}
}
