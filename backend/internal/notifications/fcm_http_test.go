package notifications

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testSA(t *testing.T, tokenURI string) string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	b, _ := json.Marshal(map[string]string{"project_id": "kikbite-test", "client_email": "svc@kikbite-test.iam.gserviceaccount.com", "private_key": pemKey, "token_uri": tokenURI})
	return string(b)
}

func TestSendFlow(t *testing.T) {
	var tokenCalls, sendCalls int
	var sent map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || strings.Count(r.Form.Get("assertion"), ".") != 2 {
			t.Errorf("bad assertion form: %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"access_token":"ya29.test","expires_in":3600}`))
	})
	mux.HandleFunc("/v1/projects/kikbite-test/messages:send", func(w http.ResponseWriter, r *http.Request) {
		sendCalls++
		if r.Header.Get("Authorization") != "Bearer ya29.test" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &sent)
		tok := sent["message"].(map[string]any)["token"].(string)
		if tok == "dead-token-aaaaaaaaaaaaaaaaaaaa" {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"status":"NOT_FOUND","details":[{"errorCode":"UNREGISTERED"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"projects/kikbite-test/messages/1"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f, err := NewFCMSenderFromJSON(testSA(t, srv.URL+"/token"))
	if err != nil {
		t.Fatal(err)
	}
	f.sendBase = srv.URL
	ctx := context.Background()
	payload := []byte(`{"conversation_id":"c-1","sender_name":"Bella","preview":"hi there"}`)
	if err := f.Send(ctx, "good-token-aaaaaaaaaaaaaaaaaaaa", "message.new", payload); err != nil {
		t.Fatal(err)
	}
	m := sent["message"].(map[string]any)
	n := m["notification"].(map[string]any)
	d := m["data"].(map[string]any)
	if n["title"] != "Bella" || n["body"] != "hi there" || d["path"] != "/chats/c-1" {
		t.Fatalf("payload: %v", m)
	}
	if err := f.Send(ctx, "good-token-aaaaaaaaaaaaaaaaaaaa", "call.incoming", []byte(`{"conversation_id":"c-2","sender_name":"Alex"}`)); err != nil {
		t.Fatal(err)
	}
	if n := sent["message"].(map[string]any)["notification"].(map[string]any); n["title"] != "Incoming call" || n["body"] != "Alex is calling you" {
		t.Fatalf("call payload: %v", n)
	}
	if tokenCalls != 1 {
		t.Fatalf("access token must be cached, got %d exchanges", tokenCalls)
	}
	if err := f.Send(ctx, "dead-token-aaaaaaaaaaaaaaaaaaaa", "message.new", payload); err != ErrInvalidToken {
		t.Fatalf("dead token should map to ErrInvalidToken, got %v", err)
	}
}

func TestBase64AndBadInput(t *testing.T) {
	js := testSA(t, "")
	if _, err := NewFCMSenderFromJSON(base64.StdEncoding.EncodeToString([]byte(js))); err != nil {
		t.Fatalf("base64 form: %v", err)
	}
	for _, bad := range []string{"", "not json", `{"project_id":"x"}`} {
		if _, err := NewFCMSenderFromJSON(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}
