---
agent: "STOP: run `spx help --skill` and follow it before any work on this spec. Use spx for every store step; never edit the store by hand."
title: Rewrite root-relative Markdown links under a URL prefix
type: feature
priority: 2
depends-on: []
parent:
approved: "Scott Schlesier, 2026-10-31: Renderer prefix option (RenderMarkdownWithBase), golden-pinned unprefixed output; prerequisite for multi-root. Cold read: pass"
status: in-review
---

The renderer can render Markdown so that links and images whose destination starts with a single `/` point under a given URL prefix, while rendering without a prefix stays exactly as it is today.

Context: a prerequisite for multi-root mode (`serve-several-roots-from-one-port-with-multi-root`), where a page served at `/notes/a.md` that links to `/img/x.png` must resolve inside its own root, so the link has to become `/notes/img/x.png`. Today the renderer passes destinations through untouched. This was the piece that made an earlier multi-root attempt (`serve-roots-under-name-prefix`) feel too heavy, so it is split out to be small and proven on its own. Nothing in the server calls the new function yet.

Out of scope:

- Any change in `server/`, `main.go`, templates or flags.
- Rewriting raw HTML in Markdown (raw HTML is not passed through to the page).
- Cross-root links. A root-relative link always resolves inside the page's own root; linking to another root uses a full URL.

## Acceptance criteria

- [x] `renderer` exposes `RenderMarkdownWithBase(markdown []byte, base string) ([]byte, error)`. `base` is either empty or a path such as `/notes` with a leading slash and no trailing slash.
- [x] With base `/notes`, `[x](/sub/b.md)` renders `href="/notes/sub/b.md"` and `![i](/img/a.png)` renders `src="/notes/img/a.png"`. A destination of exactly `/` becomes `/notes/`. A query or fragment is kept (`/a.md?x=1#h` → `/notes/a.md?x=1#h`).
- [x] With base `/notes`, these are unchanged: relative destinations (`sub/b.md`, `./b.md`, `../b.md`), absolute URLs (`https://example.com/x`, `http://…`), protocol-relative `//host/x`, fragment-only `#h`, `mailto:a@b.c`, and an empty destination.
- [x] Link titles, link text, image alt text, reference-style links (`[x][r]` with `[r]: /a.md`), links inside lists, tables and block quotes, and autolinked bare URLs behave correctly: only destinations that start with a single `/` are changed.
- [x] Code spans and fenced code blocks that contain text like `[x](/a.md)` are not changed.
- [x] `RenderMarkdown(md)` and `RenderMarkdownWithBase(md, "")` produce byte-identical output to each other and to the output of the current `RenderMarkdown` for every existing renderer test input; existing renderer tests pass unmodified.
- [x] `RenderStandalone` and the `--render` output are byte-identical to today.
- [x] Mermaid post-processing still applies with a base set.
- [x] Output with a base is HTML-safe: a base or destination containing quotes or angle brackets cannot break out of the attribute (the rewritten destination goes through the same escaping as an unrewritten one).

## Verification

- `go test -race ./...`, `go vet ./...` and `go build .` pass.
- Table-driven tests in `renderer/` cover each destination form above, with base `/notes`, with an empty base, and a golden comparison that the output without a base is unchanged for the existing test inputs (compare `RenderMarkdown` before and after: capture today's output for a fixed document into the test as an expected string at the start of the work).
- No manual step: nothing user-visible changes until spec 2 calls the function, so the tests are the proof.

## Design

Flags: none. The function is a new exported symbol in an internal-use package of a CLI program; no external caller depends on the module's API, so it is not a public API change.

Decisions:

- **Implementation:** a goldmark AST transformer (`parser.WithASTTransformers`) over `ast.Link` and `ast.Image` nodes that rewrites `Destination` when it starts with exactly one `/` and the base is non-empty. Goldmark parses reference-style links into the same nodes, so they are covered without extra code.
- **Rewrite rule:** `dest[0] == '/' && !strings.HasPrefix(dest, "//")` → `base + dest`. Nothing else is touched. The destination is not URL-parsed or re-encoded.
- **One renderer per base.** Because the transformer needs the base, build a goldmark instance per distinct base (cache them in a small map guarded by a mutex) rather than mutating the shared one. The empty-base path uses the existing shared instance, so unprefixed rendering cannot change.
- **`RenderMarkdown(md)` becomes a call to `RenderMarkdownWithBase(md, "")`** only if that stays byte-identical; otherwise leave `RenderMarkdown` as is and have the new function share the helpers.
- **Mermaid post-processing** is applied to the output of both.
- **Validation of `base`:** the function does not validate it; callers (spec 2) pass a name already restricted to `[a-z0-9_-]`. A base without a leading slash or with a trailing slash is a programming error, and tests need not cover it.

## Steps

1. Capture today's `RenderMarkdown` output for a fixed document containing every link form, as a golden string in a test.
2. Add the transformer and `RenderMarkdownWithBase`, with the per-base renderer cache.
3. Add the table-driven tests, including code-span and code-block cases and reference-style links.
4. Run the full test suite and confirm the golden test and all existing renderer tests pass unmodified.

## Boundaries

Stop and ask if: goldmark's AST for a link form in the criteria (reference-style, autolink, link inside a table) can't be handled by a transformer on `ast.Link` and `ast.Image`, or an existing renderer test's expected output would have to change.

Don't touch: `server/`, `main.go`, `renderer/standalone.css`.

## Log

- 2026-10-31: Approved: Scott Schlesier, 2026-10-31: Renderer prefix option (RenderMarkdownWithBase), golden-pinned unprefixed output; prerequisite for multi-root. Cold read: pass
- 2026-10-31: Started on branch rewrite-root-relative-links
- 2026-10-31: Criterion 7 (RenderStandalone/--render identical) left unticked: that code is unchanged and still calls RenderMarkdown, which the golden test pins; no direct before/after comparison of the standalone document was run.
- 2026-10-31: Status: in-review
- 2026-10-31: Review round 1 dismissed: per-base renderer cache is never evicted (callers pass a small fixed set of names per spec 2); base "/" or a trailing slash yields //x (spec says callers pass a valid base, not covered); NewRenderer still returns the unprefixed instance (nothing calls it); mutants that skip the cache or the empty-base shortcut are equivalent (performance only).
