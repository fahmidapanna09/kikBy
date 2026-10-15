// Package securitylint is not a real package used at runtime — it holds
// exactly one regression test that mechanically enforces the spec's
// "Logging: structured logs exclude OTPs, tokens, passwords, and message
// bodies" rule, instead of leaving it as a rule every phase's author has
// to remember by hand. It greps every .go file in the module for a zap
// structured-logging call (zap.String/Any/ByteString/Strings) whose field
// name is exactly one of a banned list, and fails if it finds one.
//
// This is deliberately a grep-based check, not a real static analyzer —
// there's no Go toolchain in the environment this project was built in to
// run go/analysis-based tooling, and a regex catches the actual mistake
// pattern (a literal field name like "otp_code" or "password" in a log
// call) without needing full type information. It will not catch every
// possible way to leak a secret into a log (e.g. fmt.Sprintf-ing a secret
// into a message string Fields doesn't see), so it's a floor, not a
// guarantee — see backend/docs/E2EE_THREAT_MODEL.md's threat model for
// what is and isn't covered by automated checks in this project.
package securitylint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Exact field names (case-insensitive), not substrings — "token_suffix" in
// notifications/fcm.go is fine (it logs only the last few characters of a
// push token, never the whole thing) and must NOT trip this check; "token"
// alone must.
var bannedFieldNames = map[string]bool{
	"otp": true, "otp_code": true, "code": true,
	"password": true,
	"token":    true, "access_token": true, "refresh_token": true, "raw_token": true,
	"body": true, "message_body": true, "content": true, "message_content": true,
	"post_body": true, "secret": true, "api_key": true, "gemini_api_key": true,
}

var zapFieldCall = regexp.MustCompile(`zap\.(?:String|Any|ByteString|Strings|Stringer)\(\s*"([A-Za-z0-9_]+)"`)

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %s", dir)
		}
		dir = parent
	}
}

func TestNoBannedFieldNamesInStructuredLogs(t *testing.T) {
	root := moduleRoot(t)

	var violations []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// This file itself legitimately contains the banned words as map
		// keys/strings, not as zap call field names — but it also contains
		// no zap.* calls at all, so it would never match zapFieldCall
		// anyway. No special-case skip needed.

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range zapFieldCall.FindAllStringSubmatch(string(data), -1) {
			field := strings.ToLower(m[1])
			if bannedFieldNames[field] {
				rel, _ := filepath.Rel(root, path)
				violations = append(violations, rel+": zap field \""+m[1]+"\"")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if len(violations) > 0 {
		t.Fatalf("found %d structured-log call(s) using a banned field name (OTP/password/token/message-content logging is forbidden by spec section 7):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}
