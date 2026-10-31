package renderer

import (
	"strings"
	"testing"
)

func TestRenderMarkdownWithBase(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []string // substrings that must appear
		not  []string // substrings that must not appear
	}{
		{"link", "[x](/sub/b.md)", []string{`href="/notes/sub/b.md"`}, nil},
		{"image", "![i](/img/a.png)", []string{`src="/notes/img/a.png"`}, nil},
		{"root", "[x](/)", []string{`href="/notes/"`}, nil},
		{"query and fragment", "[x](/a.md?x=1#h)", []string{`href="/notes/a.md?x=1#h"`}, nil},
		{"relative", "[a](sub/b.md) [b](./b.md) [c](../b.md)", []string{`href="sub/b.md"`, `href="./b.md"`, `href="../b.md"`}, []string{"/notes"}},
		{"absolute url", "[a](https://example.com/x) [b](http://example.com/x)", []string{`href="https://example.com/x"`, `href="http://example.com/x"`}, []string{"/notes"}},
		{"protocol relative", "[a](//host/x)", []string{`href="//host/x"`}, []string{"/notes"}},
		{"fragment only", "[a](#h)", []string{`href="#h"`}, []string{"/notes"}},
		{"mailto", "[a](mailto:a@b.c)", []string{`href="mailto:a@b.c"`}, []string{"/notes"}},
		{"empty destination", "[a]()", []string{`href=""`}, []string{"/notes"}},
		{"title", `[x](/a.md "T")`, []string{`href="/notes/a.md" title="T"`}, nil},
		{"link text and alt untouched", "[/a.md](/b.md) ![/c.png](/d.png)", []string{`>/a.md</a>`, `alt="/c.png"`, `href="/notes/b.md"`, `src="/notes/d.png"`}, nil},
		{"reference style", "[x][r]\n\n[r]: /a.md\n", []string{`href="/notes/a.md"`}, nil},
		{"list", "- item [y](/a.md)", []string{`href="/notes/a.md"`}, nil},
		{"table", "| a |\n|---|\n| [x](/a.md) |\n", []string{`href="/notes/a.md"`}, nil},
		{"block quote", "> [x](/a.md)", []string{`href="/notes/a.md"`}, nil},
		{"autolink", "https://example.com/x", []string{`href="https://example.com/x"`}, []string{"/notes"}},
		{"angle autolink", "<https://example.com/x>", []string{`href="https://example.com/x"`}, []string{"/notes"}},
		{"bare path is not a link", "see /a.md and www.example.com/x", []string{"see /a.md", `href="http://www.example.com/x"`}, []string{"/notes"}},
		{"code span", "`[x](/a.md)`", []string{"[x](/a.md)"}, []string{"/notes"}},
		{"fenced code", "```\n[x](/a.md)\n```\n", []string{"[x](/a.md)"}, []string{"/notes"}},
		{"mermaid", "```mermaid\ngraph TD; A-->B;\n```\n[x](/a.md)", []string{`<div class="mermaid">graph TD; A--&gt;B;</div>`, `href="/notes/a.md"`}, []string{"<pre>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderMarkdownWithBase([]byte(tt.md), "/notes")
			if err != nil {
				t.Fatal(err)
			}
			s := string(got)
			for _, w := range tt.want {
				if !strings.Contains(s, w) {
					t.Errorf("missing %q in %s", w, s)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(s, n) {
					t.Errorf("unexpected %q in %s", n, s)
				}
			}
		})
	}
}

func TestRenderMarkdownWithBaseEmptyMatchesRenderMarkdown(t *testing.T) {
	a, err := RenderMarkdown([]byte(goldenDoc))
	if err != nil {
		t.Fatal(err)
	}
	b, err := RenderMarkdownWithBase([]byte(goldenDoc), "")
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) || string(a) != goldenHTML {
		t.Errorf("empty base output differs from RenderMarkdown/golden")
	}
}

func TestRenderMarkdownWithBaseEscapesAttributes(t *testing.T) {
	got, err := RenderMarkdownWithBase([]byte(`[x](/a"><script>alert(1)</script>)`), `/n"><b>`)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "<script>") || strings.Contains(s, "<b>") || strings.Contains(s, `"><`) {
		t.Errorf("attribute not escaped: %s", s)
	}
}

func TestRenderMarkdownWithBaseDistinctBases(t *testing.T) {
	a, _ := RenderMarkdownWithBase([]byte("[x](/p.md)"), "/a")
	b, _ := RenderMarkdownWithBase([]byte("[x](/p.md)"), "/b")
	if !strings.Contains(string(a), `href="/a/p.md"`) || !strings.Contains(string(b), `href="/b/p.md"`) {
		t.Errorf("bases mixed up: %s / %s", a, b)
	}
}

func TestRenderMarkdownWithBaseOnlyChangesDestinations(t *testing.T) {
	got, err := RenderMarkdownWithBase([]byte(goldenDoc), "/x")
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if s == goldenHTML {
		t.Fatal("base had no effect")
	}
	s = strings.NewReplacer(`href="/x/`, `href="/`, `src="/x/`, `src="/`).Replace(s)
	if s != goldenHTML {
		t.Errorf("output differs from unprefixed output beyond destinations:\n%s", s)
	}
}
