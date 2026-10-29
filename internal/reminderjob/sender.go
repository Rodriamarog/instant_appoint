package reminderjob

import (
	"context"

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
		// meta.Sanitise keeps the raw Graph response — which can quote the
		// app secret — out of the log line this error is on its way to.
		return "", meta.Sanitise(err)
	}

	return messageID, nil
}
