package server

import (
	"reflect"
	"testing"
)

func matchedPaths(ms []fileMatch) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Path
	}
	return out
}

func TestMatchFilesSubsequenceAndCase(t *testing.T) {
	paths := []string{"docs/README.md", "notes/todo.md", "src/main.md"}

	got := matchedPaths(matchFiles("rdme", paths, 10))
	if want := []string{"docs/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rdme = %v, want %v", got, want)
	}

	if got := matchFiles("zzz", paths, 10); len(got) != 0 {
		t.Errorf("zzz = %v, want no matches", got)
	}
}

func TestMatchFilesRanking(t *testing.T) {
	tests := []struct {
		query string
		paths []string
		first string
	}{
		{"readme", []string{"docs/old-readme-notes.md", "README.md"}, "README.md"},
		{"ghi", []string{"guides/zzghizz.md", "guides/home-install.md"}, "guides/home-install.md"},
	}
	for _, tt := range tests {
		got := matchFiles(tt.query, tt.paths, 10)
		if len(got) != len(tt.paths) || got[0].Path != tt.first {
			t.Errorf("%q = %v, want %s first", tt.query, matchedPaths(got), tt.first)
		}
	}
}

func TestMatchFilesTieBreaksByPath(t *testing.T) {
	got := matchedPaths(matchFiles("a", []string{"d/a", "b/a", "c/a"}, 10))
	if want := []string{"b/a", "c/a", "d/a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tie order = %v, want %v", got, want)
	}
}

func TestMatchFilesLimit(t *testing.T) {
	var paths []string
	for _, c := range "abcdefghijklmnop" {
		paths = append(paths, string(c)+"/note.md")
	}
	if got := matchFiles("note", paths, 10); len(got) != 10 {
		t.Errorf("limit: got %d results, want 10", len(got))
	}
}

func TestMatchFilesEmptyQueryIsAlphabetical(t *testing.T) {
	got := matchFiles("", []string{"c.md", "a.md", "b/z.md", "b.md"}, 3)
	if want := []string{"a.md", "b.md", "b/z.md"}; !reflect.DeepEqual(matchedPaths(got), want) {
		t.Errorf("empty query = %v, want %v", matchedPaths(got), want)
	}
	if got[0].Matches == nil {
		t.Error("Matches must be an empty slice, not nil")
	}
}

func TestMatchFilesMatchIndexes(t *testing.T) {
	got := matchFiles("rdme", []string{"docs/README.md"}, 10)
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if want := []int{5, 8, 9, 10}; !reflect.DeepEqual(got[0].Matches, want) {
		t.Errorf("matches = %v, want %v", got[0].Matches, want)
	}
}

func TestMatchFilesIndexesAreCharacterPositions(t *testing.T) {
	// "é" is two bytes, so byte offsets would be one too high after it.
	got := matchFiles("n", []string{"é-notes.md"}, 10)
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	if want := []int{2}; !reflect.DeepEqual(got[0].Matches, want) {
		t.Errorf("matches = %v, want %v", got[0].Matches, want)
	}
}
