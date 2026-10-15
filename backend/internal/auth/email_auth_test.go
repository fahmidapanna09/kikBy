package auth

import "testing"

func TestNormalizeEmail(t *testing.T) {
	ok := map[string]string{"A@B.co": "a@b.co", "  user.name+tag@Example.COM ": "user.name+tag@example.com"}
	for in, want := range ok {
		if got := normalizeEmail(in); got != want {
			t.Errorf("normalizeEmail(%q)=%q want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "nope", "a@b", "Bob <a@b.co>", "a@b.co, c@d.co", "@b.co", "a b@c.co"} {
		if got := normalizeEmail(bad); got != "" {
			t.Errorf("normalizeEmail(%q)=%q want empty", bad, got)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, bad := range []string{"", "short1", "alllettersonly", "12345678"} {
		if validatePassword(bad) == "" {
			t.Errorf("validatePassword(%q) accepted", bad)
		}
	}
	if validatePassword("Str0ngPassw0rd") != "" {
		t.Error("good password rejected")
	}
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	long[0] = '1'
	if validatePassword(string(long)) == "" {
		t.Error("over-long password accepted")
	}
}

func TestNormalizeContactPhone(t *testing.T) {
	cases := map[string]string{"": "", "  ": "", "+880 1712-345678": "+8801712345678", "(880) 1712345678": "+8801712345678"}
	for in, want := range cases {
		got, ok := normalizeContactPhone(in)
		if !ok || got != want {
			t.Errorf("normalizeContactPhone(%q)=(%q,%v) want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"abc", "12", "+12", "++8801712345678", "880-ext-1", "+0123456789"} {
		if _, ok := normalizeContactPhone(bad); ok {
			t.Errorf("normalizeContactPhone(%q) accepted", bad)
		}
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := hashPassword("Str0ngPassw0rd")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(h, "Str0ngPassw0rd") {
		t.Error("correct password rejected")
	}
	if verifyPassword(h, "Str0ngPassw0rd!") {
		t.Error("wrong password accepted")
	}
	h2, _ := hashPassword("Str0ngPassw0rd")
	if h == h2 {
		t.Error("salt not random")
	}
	for _, junk := range []string{"", "x", "argon2id$1$2$3$a$b", "argon2id$99$19456$1$AA$AA", "bcrypt$1$2$3$4$5"} {
		if verifyPassword(junk, "x") {
			t.Errorf("junk hash %q verified", junk)
		}
	}
}

func TestResetTokenHashing(t *testing.T) {
	raw, hash, err := newResetToken()
	if err != nil || len(raw) < 40 || hash != hashResetToken(raw) || raw == hash {
		t.Fatalf("bad token: %q %q %v", raw, hash, err)
	}
	r2, _, _ := newResetToken()
	if raw == r2 {
		t.Error("tokens repeat")
	}
}
