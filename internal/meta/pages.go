package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// The Messenger and Instagram half of the client.
//
// Both channels are reached through a Facebook Page: a Page access token
// sends Messenger messages as the Page and Instagram messages as the
// Instagram professional account linked to it, through the same
// /{page-id}/messages endpoint. That is why one Facebook Login connects
// both, and why there is no Instagram-specific token here at all.
//
// These calls use one Graph version throughout, unlike the WhatsApp half,
// because none of them has a working predecessor to stay byte-identical to.
const pagesVersion = "v25.0"

// PageMessagingFields are the webhook fields a connected Page is subscribed
// to. messages carries what customers write (and, for an Instagram account
// linked to the Page, what they write there), message_echoes what the
// business itself sends from Messenger or Instagram, so the assistant reads
// a human reply as the business's turn.
var PageMessagingFields = []string{"messages", "messaging_postbacks", "message_echoes"}

// LongLivedUserToken trades a user access token for one that lasts about 60
// days.
//
// The point is not the 60 days: a Page token read with a long-lived user
// token has no expiry at all, while one read with a short-lived token dies
// with it an hour later.
func (c *Client) LongLivedUserToken(ctx context.Context, userToken string) (string, error) {
	body, err := c.get(ctx, pagesVersion, "oauth/access_token", url.Values{
		"grant_type":        {"fb_exchange_token"},
		"client_id":         {c.appID},
		"client_secret":     {c.appSecret},
		"fb_exchange_token": {userToken},
	}, "")
	if err != nil {
		return "", err
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("meta: parse long-lived token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", &APIError{Status: http.StatusOK, Message: "the long-lived token exchange returned no access_token", Body: string(body)}
	}

	return parsed.AccessToken, nil
}

// Page is a Facebook Page the person who logged in manages, with the
// Instagram professional account linked to it when there is one.
type Page struct {
	ID          string
	Name        string
	AccessToken string

	// Instagram is nil when the Page has no linked Instagram professional
	// account — a personal Instagram account cannot be linked, and cannot
	// receive messages through the API at all.
	Instagram *InstagramAccount
}

// InstagramAccount is an Instagram professional account linked to a Page.
type InstagramAccount struct {
	ID       string
	Username string
	Name     string
}

// Pages lists the Pages the user granted this app, with their tokens.
func (c *Client) Pages(ctx context.Context, userToken string) ([]Page, error) {
	body, err := c.get(ctx, pagesVersion, "me/accounts", url.Values{
		"fields": {"id,name,access_token,instagram_business_account{id,username,name}"},
		"limit":  {"100"},
	}, userToken)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			AccessToken string `json:"access_token"`
			Instagram   *struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Name     string `json:"name"`
			} `json:"instagram_business_account"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("meta: parse me/accounts response: %w", err)
	}

	pages := make([]Page, 0, len(parsed.Data))
	for _, raw := range parsed.Data {
		// A Page without a token is one the user can see but this app was
		// not granted; it cannot be answered from, so it is not offered.
		if raw.ID == "" || raw.AccessToken == "" {
			continue
		}

		page := Page{ID: raw.ID, Name: raw.Name, AccessToken: raw.AccessToken}
		if raw.Instagram != nil && raw.Instagram.ID != "" {
			page.Instagram = &InstagramAccount{
				ID:       raw.Instagram.ID,
				Username: raw.Instagram.Username,
				Name:     raw.Instagram.Name,
			}
		}
		pages = append(pages, page)
	}

	return pages, nil
}

// SubscribePage subscribes this app to a Page's webhook events.
//
// Like the WABA subscription, this is what makes Meta deliver anything:
// without it the Page is connected and deaf. It also covers the linked
// Instagram account, whose messages Meta delivers to apps subscribed to the
// Page.
func (c *Client) SubscribePage(ctx context.Context, pageID, pageToken string, fields []string) error {
	query := url.Values{"subscribed_fields": {strings.Join(fields, ",")}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.url(pagesVersion, pageID+"/subscribed_apps", query), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pageToken)

	body, err := c.do(req)
	if err != nil {
		return err
	}

	return requireSuccess(body, "the Page webhook subscription was not confirmed")
}

// UnsubscribePage stops this app's webhook events for a Page.
func (c *Client) UnsubscribePage(ctx context.Context, pageID, pageToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.url(pagesVersion, pageID+"/subscribed_apps", nil), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pageToken)

	body, err := c.do(req)
	if err != nil {
		return err
	}

	return requireSuccess(body, "the Page webhook unsubscription was not confirmed")
}

func requireSuccess(body []byte, refusal string) error {
	var parsed struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("meta: parse subscribed_apps response: %w", err)
	}
	if !parsed.Success {
		return &APIError{Status: http.StatusOK, Message: refusal, Body: string(body)}
	}

	return nil
}

// MaxQuickReplyTitle is how long a Messenger or Instagram quick reply's
// title may be, in characters.
const MaxQuickReplyTitle = 20

type pageSendRequest struct {
	Recipient     pageRecipient `json:"recipient"`
	MessagingType string        `json:"messaging_type"`
	Message       pageMessage   `json:"message"`
}

type pageRecipient struct {
	ID string `json:"id"`
}

type pageMessage struct {
	Text         string           `json:"text"`
	QuickReplies []pageQuickReply `json:"quick_replies,omitempty"`
}

type pageQuickReply struct {
	ContentType string `json:"content_type"`
	Title       string `json:"title"`
	Payload     string `json:"payload"`
}

// SendPageMessage sends text from a Page to a Messenger or Instagram
// customer and returns Meta's id for the message.
//
// recipientID is the customer's page-scoped id (PSID) on Messenger or their
// Instagram-scoped id (IGSID) on Instagram — the id the webhook named them
// by. Buttons become quick replies, whose payload Meta hands back untouched
// when one is tapped.
//
// messaging_type RESPONSE: everything this app sends on these channels is an
// answer inside the 24-hour window the customer's own message opened. A
// message outside it would need a message tag, and none of the tags Meta
// still offers covers an appointment reminder on Instagram, which is why
// reminders stay on WhatsApp.
func (c *Client) SendPageMessage(ctx context.Context, pageID, pageToken, recipientID, text string, buttons []Button) (string, error) {
	message := pageMessage{Text: text}
	for _, button := range buttons {
		message.QuickReplies = append(message.QuickReplies, pageQuickReply{
			ContentType: "text",
			Title:       button.Title,
			Payload:     button.ID,
		})
	}

	body, err := c.post(ctx, pagesVersion, pageID+"/messages", pageSendRequest{
		Recipient:     pageRecipient{ID: recipientID},
		MessagingType: "RESPONSE",
		Message:       message,
	}, pageToken)
	if err != nil {
		return "", err
	}

	var parsed struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("meta: parse page send response: %w", err)
	}
	if parsed.MessageID == "" {
		return "", &APIError{Status: http.StatusOK, Message: "the send response carried no message id", Body: string(body)}
	}

	return parsed.MessageID, nil
}
