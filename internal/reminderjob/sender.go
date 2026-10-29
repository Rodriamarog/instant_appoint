package reminderjob

import (
	"context"
	"errors"
	"fmt"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/internal/reminders"
)

// Sender delivers a reminder through Meta's Cloud API.
type Sender struct {
	client *meta.Client
}

// NewSender returns a sender that sends through client.
func NewSender(client *meta.Client) *Sender {
	return &Sender{client: client}
}

// SendTemplate sends one approved template from the account's number.
//
// A reminder cannot be free text. Outside the 24-hour window a customer's own
// message opens, Meta delivers nothing but an approved template, so the three
// values — name, date, time — travel as positional body parameters and the
// words around them are whatever the business had approved.
func (s *Sender) SendTemplate(ctx context.Context, account reminders.Account, to, templateName, languageCode string, bodyParameters []string) (string, error) {
	messageID, err := s.client.SendTemplate(ctx,
		account.PhoneNumberID, account.AccessToken, to, templateName, languageCode, bodyParameters)
	if err != nil {
		return "", sanitise(err)
	}

	return messageID, nil
}

// sanitise strips the raw Graph response out of a Meta failure before it
// travels any further.
//
// meta.APIError keeps the whole response body, and its Error() falls back to
// printing it when Meta sent no message. That body can carry the app secret,
// and this error is on its way to a log line. Only Meta's own explanation and
// its codes come out.
func sanitise(err error) error {
	var apiErr *meta.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	message := apiErr.Message
	if message == "" {
		message = "Meta refused the send and explained nothing"
	}

	return fmt.Errorf("meta: %s (http %d, code %d/%d)", message, apiErr.Status, apiErr.Code, apiErr.Subcode)
}
