package server

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePath(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.Abs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	fs := &FileService{Root: root, MaxUploadBytes: 1024}

	tests := []struct {
		name    string
		input   string
		wantErr bool
		// suffix that the absolute result must end with (using filepath.Separator).
		wantSuffix string
	}{
		{"empty", "", false, ""},
		{"plain file", "report.txt", false, "report.txt"},
		{"nested dir", "sub/deep/file.log", false, filepath.Join("sub", "deep", "file.log")},
		{"backslash normalized", `sub\file.log`, false, filepath.Join("sub", "file.log")},

		{"dot-dot escape", "../etc/hosts", true, ""},
		{"deep dot-dot", "a/b/../../../escape", true, ""},
		{"only dot-dot", "..", true, ""},
		{"absolute unix", "/etc/passwd", true, ""},
		{"absolute windows", `C:\Windows\System32\drivers\etc\hosts`, true, ""},
		{"windows drive lowercase", `c:/foo`, true, ""},
		{"empty segment with dot-dot", "./../escape", true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fs.SafePath(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("SafePath(%q): expected error, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SafePath(%q): unexpected error %v", tc.input, err)
			}
			if !strings.HasPrefix(got, root) {
				t.Fatalf("SafePath(%q) = %q, must be under %q", tc.input, got, root)
			}
			if tc.wantSuffix != "" && !strings.HasSuffix(got, tc.wantSuffix) {
				t.Fatalf("SafePath(%q) = %q, want suffix %q", tc.input, got, tc.wantSuffix)
			}
		})
	}
}

func TestSafeBaseName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"report.txt", "report.txt"},
		{"sub/report.txt", "report.txt"},        // directory components stripped
		{`sub\report.txt`, "report.txt"},        // windows separators
		{"", ""},
		{".", ""},
		{"..", ""},
		{".env", ".env"}, // hidden file is fine; we just block path components
	}
	for _, tc := range tests {
		if got := safeBaseName(tc.in); got != tc.want {
			t.Errorf("safeBaseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
