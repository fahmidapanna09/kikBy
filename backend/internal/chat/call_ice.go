package chat

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // coturn's REST-credential scheme is defined as HMAC-SHA1
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"time"
)

// IceConfig describes the STUN/TURN servers handed to call participants.
//
// STUN alone is enough for a prototype and for most same-network / home-broadband pairs.
// A TURN relay is what makes calls work between strict NATs (mobile data <-> Wi-Fi), and is the only part that costs money or a server.
// Two ways to supply TURN credentials:
//   - TURN_SECRET: a coturn `static-auth-secret`; the server mints short-lived per-user credentials (nothing long-lived reaches the browser).
//   - TURN_USERNAME + TURN_CREDENTIAL: static credentials from a hosted TURN provider.
type IceConfig struct {
	StunURLs       []string
	TurnURLs       []string
	TurnSecret     string
	TurnUsername   string
	TurnCredential string
	TTL            time.Duration
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// IceConfigFromEnv reads STUN_URLS (default: Google's public STUN), TURN_URLS, TURN_SECRET, TURN_USERNAME, TURN_CREDENTIAL, TURN_TTL (default 1h).
func IceConfigFromEnv() IceConfig {
	c := IceConfig{
		StunURLs:       splitList(os.Getenv("STUN_URLS")),
		TurnURLs:       splitList(os.Getenv("TURN_URLS")),
		TurnSecret:     os.Getenv("TURN_SECRET"),
		TurnUsername:   os.Getenv("TURN_USERNAME"),
		TurnCredential: os.Getenv("TURN_CREDENTIAL"),
		TTL:            time.Hour,
	}
	if len(c.StunURLs) == 0 {
		c.StunURLs = []string{"stun:stun.l.google.com:19302"}
	}
	if d, err := time.ParseDuration(os.Getenv("TURN_TTL")); err == nil && d > 0 {
		c.TTL = d
	}
	return c
}

// ServersFor returns the iceServers list for one user.
func (c IceConfig) ServersFor(userID string, now time.Time) []IceServer {
	var out []IceServer
	if len(c.StunURLs) > 0 {
		out = append(out, IceServer{URLs: c.StunURLs})
	}
	if len(c.TurnURLs) == 0 {
		return out
	}
	switch {
	case c.TurnSecret != "":
		ttl := c.TTL
		if ttl <= 0 {
			ttl = time.Hour
		}
		username := strconv.FormatInt(now.Add(ttl).Unix(), 10) + ":" + userID
		mac := hmac.New(sha1.New, []byte(c.TurnSecret))
		mac.Write([]byte(username))
		out = append(out, IceServer{URLs: c.TurnURLs, Username: username, Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil))})
	case c.TurnUsername != "" && c.TurnCredential != "":
		out = append(out, IceServer{URLs: c.TurnURLs, Username: c.TurnUsername, Credential: c.TurnCredential})
	}
	return out
}
