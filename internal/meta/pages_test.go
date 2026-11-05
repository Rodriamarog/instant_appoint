package meta_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
)

func TestLongLivedUserTokenSendsTheExchangeGrant(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusOK, `{"access_token":"LONG","token_type":"bearer"}`
	})

	token, err := stub.client().LongLivedUserToken(context.Background(), "SHORT")
	if err != nil {
		t.Fatalf("LongLivedUserToken: %v", err)
	}
	if token != "LONG" {
		t.Errorf("token = %q, want LONG", token)
	}

	call := stub.only(t)
	if call.path != "/v25.0/oauth/access_token" {
		t.Errorf("path = %q", call.path)
	}
	for key, want := range map[string]string{
		"grant_type":        "fb_exchange_token",
		"client_id":         "APPID",
		"client_secret":     "APPSECRET",
		"fb_exchange_token": "SHORT",
	} {
		if got := call.query[key]; len(got) != 1 || got[0] != want {
			t.Errorf("query %s = %v, want %q", key, got, want)
		}
	}
}

func TestPagesReadsTheLinkedInstagramAccountAndDropsPagesWithoutAToken(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusOK, `{"data":[
			{"id":"P1","name":"Clínica Sol","access_token":"PT1","instagram_business_account":{"id":"IG1","username":"clinicasol","name":"Clínica Sol"}},
			{"id":"P2","name":"Sin Instagram","access_token":"PT2"},
			{"id":"P3","name":"Sin permiso"}
		]}`
	})

	pages, err := stub.client().Pages(context.Background(), "USER")
	if err != nil {
		t.Fatalf("Pages: %v", err)
	}

	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2: %+v", len(pages), pages)
	}
	if pages[0].Instagram == nil || pages[0].Instagram.ID != "IG1" || pages[0].Instagram.Username != "clinicasol" {
		t.Errorf("first page's Instagram = %+v", pages[0].Instagram)
	}
	if pages[1].Instagram != nil {
		t.Errorf("a page with no linked account came back with one: %+v", pages[1].Instagram)
	}

	call := stub.only(t)
	if call.path != "/v25.0/me/accounts" || call.header.Get("Authorization") != "Bearer USER" {
		t.Errorf("request = %s with %q", call.path, call.header.Get("Authorization"))
	}
}

func TestSubscribePagePutsTheFieldsInTheQueryAndRequiresSuccess(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusOK, `{"success":true}`
	})

	if err := stub.client().SubscribePage(context.Background(), "P1", "PT", meta.PageMessagingFields); err != nil {
		t.Fatalf("SubscribePage: %v", err)
	}

	call := stub.only(t)
	if call.method != http.MethodPost || call.path != "/v25.0/P1/subscribed_apps" {
		t.Errorf("request = %s %s", call.method, call.path)
	}
	if got := call.query["subscribed_fields"]; len(got) != 1 || got[0] != "messages,messaging_postbacks,message_echoes" {
		t.Errorf("subscribed_fields = %v", got)
	}
	if call.body != "" {
		t.Errorf("body = %q, want none", call.body)
	}

	refused := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusOK, `{"success":false}`
	})
	if err := refused.client().SubscribePage(context.Background(), "P1", "PT", meta.PageMessagingFields); err == nil {
		t.Error("a success:false subscription was reported as working")
	}
}

func TestSendPageMessageSendsQuickRepliesWithTheirPayloads(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusOK, `{"recipient_id":"PSID","message_id":"m_1"}`
	})

	id, err := stub.client().SendPageMessage(context.Background(), "P1", "PT", "PSID", "¿Confirmas?", []meta.Button{
		{ID: "confirm:A1", Title: "Confirmar"},
		{ID: "cancel:A1", Title: "Cancelar"},
	})
	if err != nil {
		t.Fatalf("SendPageMessage: %v", err)
	}
	if id != "m_1" {
		t.Errorf("id = %q", id)
	}

	call := stub.only(t)
	if call.path != "/v25.0/P1/messages" || call.header.Get("Authorization") != "Bearer PT" {
		t.Errorf("request = %s with %q", call.path, call.header.Get("Authorization"))
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(call.body), &sent); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	want := `{"message":{"quick_replies":[{"content_type":"text","payload":"confirm:A1","title":"Confirmar"},{"content_type":"text","payload":"cancel:A1","title":"Cancelar"}],"text":"¿Confirmas?"},"messaging_type":"RESPONSE","recipient":{"id":"PSID"}}`
	got, _ := json.Marshal(sent)
	if string(got) != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestSendPageMessageWithoutButtonsSendsNoQuickReplies(t *testing.T) {
	stub := newGraphStub(t, func(*http.Request) (int, string) {
		return http.StatusOK, `{"recipient_id":"IGSID","message_id":"m_2"}`
	})

	if _, err := stub.client().SendPageMessage(context.Background(), "P1", "PT", "IGSID", "Hola", nil); err != nil {
		t.Fatalf("SendPageMessage: %v", err)
	}

	if body := stub.only(t).body; body != `{"recipient":{"id":"IGSID"},"messaging_type":"RESPONSE","message":{"text":"Hola"}}` {
		t.Errorf("body = %s", body)
	}
}
