package server

import (
	"sort"
	"unicode/utf8"

	"github.com/sahilm/fuzzy"
)

// fileMatch is one picker result. Matches holds the character (not byte) positions
// in Path that the query matched, for the client to emphasize.
type fileMatch struct {
	Path    string
	Matches []int
}

// matchFiles returns up to limit paths matching query, best first. Matching is
// fzf-style: the query's characters must appear in order, case-insensitively.
// Equal scores break by path. An empty query returns the first paths
// alphabetically.
func matchFiles(query string, paths []string, limit int) []fileMatch {
	if query == "" {
		sorted := append([]string(nil), paths...)
		sort.Strings(sorted)
		if len(sorted) > limit {
			sorted = sorted[:limit]
		}
		out := make([]fileMatch, len(sorted))
		for i, p := range sorted {
			out[i] = fileMatch{Path: p, Matches: []int{}}
		}
		return out
	}

	found := fuzzy.Find(query, paths)
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Score != found[j].Score {
			return found[i].Score > found[j].Score
		}
		return found[i].Str < found[j].Str
	})
	if len(found) > limit {
		found = found[:limit]
	}
	out := make([]fileMatch, len(found))
	for i, m := range found {
		out[i] = fileMatch{Path: m.Str, Matches: runeIndexes(m.Str, m.MatchedIndexes)}
	}
	return out
}

// runeIndexes converts byte offsets into s to character positions.
func runeIndexes(s string, byteOffsets []int) []int {
	out := make([]int, len(byteOffsets))
	for i, off := range byteOffsets {
		out[i] = utf8.RuneCountInString(s[:off])
	}
	return out
}
