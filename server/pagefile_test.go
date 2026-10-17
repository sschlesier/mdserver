package server

import "testing"

func TestIsPageFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"a.md", true},
		{"docs/A.MD", true},
		{"b.Md", true},
		{"a.markdown", false},
		{"a.png", false},
		{"md", false},
		{"a.md.txt", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isPageFile(tt.name); got != tt.want {
			t.Errorf("isPageFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
