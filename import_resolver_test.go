package main

import "testing"

func TestImportVersionParsing(t *testing.T) {
	tests := []struct {
		input      string
		wantSource string
		wantVer    string
	}{
		{"github.com/user/repo", "github.com/user/repo", ""},
		{"github.com/user/repo@main", "github.com/user/repo", "main"},
		{"github.com/user/repo@v1.2.3", "github.com/user/repo", "v1.2.3"},
		{"github.com/user/repo@latest", "github.com/user/repo", "latest"},
	}

	for _, tt := range tests {
		spec, err := ParseImportSource(tt.input)
		if err != nil {
			t.Errorf("ParseImportSource(%q) error = %v", tt.input, err)
			continue
		}
		if spec.Source != tt.wantSource {
			t.Errorf("ParseImportSource(%q).Source = %q, want %q", tt.input, spec.Source, tt.wantSource)
		}
		if spec.Version != tt.wantVer {
			t.Errorf("ParseImportSource(%q).Version = %q, want %q", tt.input, spec.Version, tt.wantVer)
		}
	}
}
