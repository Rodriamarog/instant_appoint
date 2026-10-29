package meta_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
)

// The body Meta echoes back quotes the request, and several of these calls
// put the app secret in the query string. Three call sites used to carry
// their own copy of this rule; it lives here now, so this is where it is
// proven.
const secret = "client_secret=THE-APP-SECRET"

func refusal(message string) *meta.APIError {
	return &meta.APIError{
		Status:  400,
		Message: message,
		Code:    100,
		Subcode: 36009,
		Body:    `{"error":{"message":"` + message + `"}} ` + secret,
	}
}

func TestSafeNeverCarriesTheResponseBody(t *testing.T) {
	cases := []*meta.APIError{
		refusal("Template name already exists"),
		// The dangerous one: with no message Error() prints the whole body.
		refusal(""),
	}

	for _, apiErr := range cases {
		safe := apiErr.Safe()
		if strings.Contains(safe, secret) || strings.Contains(safe, apiErr.Body) {
			t.Errorf("Safe() = %q, which carries the response body", safe)
		}
		if !strings.Contains(safe, "http 400") || !strings.Contains(safe, "100/36009") {
			t.Errorf("Safe() = %q, want the status and both codes", safe)
		}
	}

	if got := refusal("Template name already exists").Safe(); !strings.Contains(got, "Template name already exists") {
		t.Errorf("Safe() = %q, want Meta's own explanation", got)
	}
	if got := refusal("").Safe(); got == "" {
		t.Error("Safe() is empty when Meta explained nothing, leaving a log line with no failure in it")
	}
}

// Error() is exactly what must not reach a log, which is why Sanitise exists.
func TestTheDefaultErrorStringStillPrintsTheBodySoSanitiseIsNotOptional(t *testing.T) {
	if !strings.Contains(refusal("").Error(), secret) {
		t.Skip("Error() no longer prints the body; Sanitise may be redundant")
	}

	sanitised := meta.Sanitise(fmt.Errorf("sending the reminder: %w", refusal("")))
	if strings.Contains(sanitised.Error(), secret) {
		t.Errorf("Sanitise left the body in: %q", sanitised)
	}
}

func TestSanitiseLeavesAnythingThatIsNotAMetaFailureAlone(t *testing.T) {
	original := errors.New("dial tcp: connection refused")

	if got := meta.Sanitise(original); got != original {
		t.Errorf("Sanitise returned %v, want the error unchanged", got)
	}
}

func TestExplainReturnsOnlyMetasOwnSentence(t *testing.T) {
	wrapped := fmt.Errorf("send the reply: %w", refusal("Message failed to send because more than 24 hours have passed"))

	message, ok := meta.Explain(wrapped)
	if !ok {
		t.Fatal("Explain did not see the Meta failure through the wrapping")
	}
	if message != "Message failed to send because more than 24 hours have passed" {
		t.Errorf("Explain = %q, want Meta's error.message verbatim", message)
	}

	if _, ok := meta.Explain(refusal("")); ok {
		t.Error("Explain claimed an explanation where Meta gave none, which would show a person an empty sentence")
	}
	if _, ok := meta.Explain(errors.New("dial tcp: connection refused")); ok {
		t.Error("Explain claimed an explanation for an error Meta never sent")
	}
}
