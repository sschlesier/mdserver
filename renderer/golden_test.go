package renderer

import "testing"

const goldenDoc = "# Links\n\n" +
	"[abs](/sub/b.md) ![img](/img/a.png) [root](/) [q](/a.md?x=1#h \"T\")\n\n" +
	"[rel](sub/b.md) [dot](./b.md) [up](../b.md) [url](https://example.com/x) [proto](//host/x) [frag](#h) [mail](mailto:a@b.c) []()\n\n" +
	"[ref][r] and https://example.com/auto\n\n" +
	"[r]: /ref.md\n\n" +
	"- item [l](/list.md)\n\n" +
	"> quote [q](/quote.md)\n\n" +
	"| a | b |\n|---|---|\n| [t](/table.md) | x |\n\n" +
	"`[c](/code.md)`\n\n" +
	"```\n[f](/fence.md)\n```\n\n" +
	"```mermaid\ngraph TD; A-->B;\n```\n"

const goldenHTML = "<h1 id=\"links\">Links</h1>\n<p><a href=\"/sub/b.md\">abs</a> <img src=\"/img/a.png\" alt=\"img\" /> <a href=\"/\">root</a> <a href=\"/a.md?x=1#h\" title=\"T\">q</a></p>\n<p><a href=\"sub/b.md\">rel</a> <a href=\"./b.md\">dot</a> <a href=\"../b.md\">up</a> <a href=\"https://example.com/x\">url</a> <a href=\"//host/x\">proto</a> <a href=\"#h\">frag</a> <a href=\"mailto:a@b.c\">mail</a> <a href=\"\"></a></p>\n<p><a href=\"/ref.md\">ref</a> and <a href=\"https://example.com/auto\">https://example.com/auto</a></p>\n<ul>\n<li>item <a href=\"/list.md\">l</a></li>\n</ul>\n<blockquote>\n<p>quote <a href=\"/quote.md\">q</a></p>\n</blockquote>\n<table>\n<thead>\n<tr>\n<th>a</th>\n<th>b</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td><a href=\"/table.md\">t</a></td>\n<td>x</td>\n</tr>\n</tbody>\n</table>\n<p><code>[c](/code.md)</code></p>\n<pre><code>[f](/fence.md)\n</code></pre>\n<div class=\"mermaid\">graph TD; A--&gt;B;</div>\n"

func TestRenderMarkdownGolden(t *testing.T) {
	got, err := RenderMarkdown([]byte(goldenDoc))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != goldenHTML {
		t.Errorf("output changed:\n%q", got)
	}
}
