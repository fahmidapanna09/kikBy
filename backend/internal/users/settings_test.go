package users

import "testing"

func TestValidateSettings(t *testing.T) {
	ok := []string{`{}`, `{"privacy":{"readReceipts":false}}`, ` {"a":1} `}
	bad := []string{``, `[]`, `"x"`, `null`, `{`, `{"a":` + string(make([]byte, 9000)) + `}`}
	for _, s := range ok {
		if err := ValidateSettings([]byte(s)); err != nil {
			t.Errorf("%q should be valid: %v", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateSettings([]byte(s)); err == nil {
			t.Errorf("%.20q should be invalid", s)
		}
	}
}
