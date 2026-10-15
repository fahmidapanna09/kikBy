package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

func sign(secret, id, ts string, body []byte) string {
	key, _ := base64.StdEncoding.DecodeString(secret[len("whsec_"):])
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id + "." + ts + "."))
	m.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func TestVerifySvix(t *testing.T) {
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"email.bounced"}`)
	good := sign(secret, "msg_1", ts, body)

	if err := VerifySvix(secret, "msg_1", ts, good, body, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifySvix(secret, "msg_1", ts, "v1,AAAA "+good, body, now); err != nil {
		t.Fatalf("a rotated header with one good signature must pass: %v", err)
	}
	cases := map[string]error{
		"tampered body": VerifySvix(secret, "msg_1", ts, good, []byte(`{"type":"email.delivered"}`), now),
		"wrong id":      VerifySvix(secret, "msg_2", ts, good, body, now),
		"stale":         VerifySvix(secret, "msg_1", ts, good, body, now.Add(10*time.Minute)),
		"no signature":  VerifySvix(secret, "msg_1", ts, "", body, now),
		"bad secret":    VerifySvix("whsec_!!!", "msg_1", ts, good, body, now),
		"bad timestamp": VerifySvix(secret, "msg_1", "x", good, body, now),
		"wrong version": VerifySvix(secret, "msg_1", ts, "v2,"+good[3:], body, now),
	}
	for name, err := range cases {
		if err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

func TestSuppressionFor(t *testing.T) {
	var perm, soft, comp, deliv resendEvent
	perm.Type, perm.Data.To, perm.Data.Bounce.Type = "email.bounced", []string{"a@x.io"}, "Permanent"
	soft.Type, soft.Data.To, soft.Data.Bounce.Type = "email.bounced", []string{"b@x.io"}, "Transient"
	comp.Type, comp.Data.To = "email.complained", []string{"c@x.io"}
	deliv.Type, deliv.Data.To = "email.delivered", []string{"d@x.io"}
	if r, a := suppressionFor(perm); r != "bounced" || len(a) != 1 {
		t.Error("permanent bounce must suppress")
	}
	if r, _ := suppressionFor(soft); r != "" {
		t.Error("soft bounce must not suppress")
	}
	if r, _ := suppressionFor(comp); r != "complained" {
		t.Error("complaint must suppress")
	}
	if r, _ := suppressionFor(deliv); r != "" {
		t.Error("delivered must not suppress")
	}
}
