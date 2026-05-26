package server

import "testing"

func TestNameRegex(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"work", true},
		{"work-123", true},
		{"a", true},
		{"a_b-c", true},
		{"AlphaNum123", true},
		{"01234567890123456789012345678901", true}, // exactly 32 chars

		{"", false},                                  // empty
		{"_underscore-start", false},                 // can't lead with _
		{"-dashstart", false},                        // can't lead with -
		{"with space", false},                        // no whitespace
		{"with/slash", false},                        // no slash
		{"with\\backslash", false},                   // no backslash
		{"name.with.dots", false},                    // dots not allowed
		{"012345678901234567890123456789012", false}, // 33 chars
		{"name\nnewline", false},                     // newline injection
		{"name\x00nul", false},                       // nul byte
	}
	for _, tc := range tests {
		if got := nameRe.MatchString(tc.name); got != tc.ok {
			t.Errorf("nameRe(%q) = %v, want %v", tc.name, got, tc.ok)
		}
	}
}
