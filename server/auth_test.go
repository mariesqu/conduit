package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"equal", "abcdef", "abcdef", true},
		{"different", "abcdef", "abcdez", false},
		{"empty a", "", "abcdef", false},
		{"empty b", "abcdef", "", false},
		{"both empty", "", "", false}, // empties are rejected (no shared secret)
		{"different lengths", "abc", "abcdef", false},
		{"long match", strings.Repeat("x", 64), strings.Repeat("x", 64), true},
		{"long mismatch tail", strings.Repeat("x", 63) + "a", strings.Repeat("x", 63) + "b", false},
	}
	for _, tc := range tests {
		if got := tokenEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("tokenEqual(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestAuthorize_HeaderOnly(t *testing.T) {
	cfg := &Config{Token: "secret-token"}

	tests := []struct {
		name    string
		header  string
		query   string
		wantHdr bool
		wantQry bool
	}{
		{"correct header", "secret-token", "", true, true},
		{"wrong header", "wrong", "", false, false},
		{"correct query only", "", "secret-token", false, true},
		{"wrong query only", "", "wrong", false, false},
		{"no token at all", "", "", false, false},
		{"both correct", "secret-token", "secret-token", true, true},
		{"header correct, query wrong", "secret-token", "wrong", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := "/api/anything"
			if tc.query != "" {
				url += "?token=" + tc.query
			}
			r := httptest.NewRequest("GET", url, nil)
			if tc.header != "" {
				r.Header.Set("X-Auth-Token", tc.header)
			}
			if got := authorize(cfg, r); got != tc.wantHdr {
				t.Errorf("authorize(header=%q, query=%q) = %v, want %v", tc.header, tc.query, got, tc.wantHdr)
			}
			if got := authorizeWithQuery(cfg, r); got != tc.wantQry {
				t.Errorf("authorizeWithQuery(header=%q, query=%q) = %v, want %v", tc.header, tc.query, got, tc.wantQry)
			}
		})
	}
}
