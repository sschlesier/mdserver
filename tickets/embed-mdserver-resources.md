---
title: Embed mdserver's resources in the binary
type: task
priority: 3
depends-on: [move-mdserver-files-off-assets]
parent:
approved: "Scott Schlesier, 2026-10-11: embed under server/template, remove all fallbacks. Cold read: not run"
status: in-review
---

mdserver serves its stylesheet, page templates and favicon from files compiled into the binary,
and never reads them from disk at runtime, so a page looks the same on every machine
regardless of the working directory or what sits next to the binary.

Context: today `server/handlers.go` and `server/server.go` read `template/style.css`,
`page.html`, `directory.html`, `settings.html` and `favicon.svg` with `os.ReadFile`, first from
`<exe dir>/template/`, then from `./template/`, and fall back to inline copies (a plainer
default CSS and inline templates) when the files are missing. The result depends on where
mdserver runs: a machine with a `template/` directory beside the binary or in the working
directory renders differently from one without (observed by Scott Schlesier, 2026-10-11,
between two machines). `renderer/markdown.go` already embeds `standalone.css` for `--render`.
Found while reviewing `move-mdserver-files-off-assets`, which accepted the risk of stale
on-disk templates linking the old stylesheet path.

Out of scope:

- The `--render` stylesheet (`renderer/standalone.css`), already embedded.
- Letting users override templates or CSS. This removes that by accident, so say so in
  `CHANGES.md`; it isn't a feature to rebuild.

## Acceptance criteria

- [x] `template/style.css`, `page.html`, `directory.html`, `settings.html` and `favicon.svg`
      are embedded with `go:embed`, and no code in `server/` reads them from disk.
- [x] A test starts the server from a working directory with no `template/` folder, with the
      binary's directory also lacking one, and checks that a Markdown page, a directory index,
      `/settings`, `/_mdserver/style.css` and `/favicon.svg` serve the full embedded content
      (not a fallback).
- [x] A test places a different `template/page.html` and `style.css` in the working directory
      and checks they're ignored.
- [x] The inline fallbacks are removed (default page, directory and settings templates, default
      CSS, default favicon), since the embedded files can't be missing. The "full or default
      CSS" branch in `server/content_test.go` goes with them: the test asserts the full CSS.
- [x] `CHANGES.md` notes that on-disk `template/` files are no longer read.

## Verification

- `go test ./...`
- Manual: build, copy the binary to an empty directory, run it with `--dir <folder>
  --no-open`; pages are fully styled and match a run from the repo checkout.

## Design

- Move `template/` to `server/template/` with `git mv` and embed it with `//go:embed` in the `server`
  package (go:embed can't reach `../`). No separate `template` package.
- Template parsing happens once at startup, not per request.
- Release packaging doesn't ship `template/` (checked `.github/workflows/release.yml` and
  `scripts/`), so nothing there changes. Homebrew installs therefore render with the inline
  fallbacks today; after this change they get the full styling, which is the intended fix.
  Say so in `CHANGES.md`.
- The embedded favicon is served from memory, keeping its `Content-Type: image/svg+xml` and
  `Cache-Control: public, max-age=3600`. `Last-Modified`/range handling from `http.ServeFile`
  is not kept.

## Boundaries

Stop and ask if: a served path changes, or a response header other than the favicon's
`Last-Modified` changes. The HTML output of a page may change only by the full embedded
template replacing a fallback.

## Log

- 2026-10-11: Drafted from the review of move-mdserver-files-off-assets.
- 2026-10-11: Refined against the code. Decided: move `template/` under `server/`; fallbacks incl. settings and favicon removed; release doesn't ship `template/`.
- 2026-10-11: Approved. Scott Schlesier, 2026-10-11: embed under server/template, remove all fallbacks. Cold read: not run
- 2026-10-11: Started on branch embed-mdserver-resources
- 2026-10-11: Implemented. The stylesheet and favicon go through `http.ServeContent` with the process start time as the modification time, so `Last-Modified`, `Accept-Ranges` and conditional requests stay as `http.ServeFile` gave them (the Design allowed dropping them for the favicon; nothing is dropped). Removed `TestTemplateFilesLinkStylesheet`: it read `../template/` from disk, and the handler test above it now covers the embedded templates.
- 2026-10-11: Revert check: both new tests fail against the pre-change code (fallback CSS and favicon served; working-directory decoys served). The page-template checks alone pass there, since the old inline page templates had the same markup.
- 2026-10-11: Manual check: binary built into an empty directory and run with `--dir tickets --no-open`; every page, the stylesheet and the favicon are byte-identical to a `go run .` from the checkout, and the CSS matches `server/template/style.css`.
- 2026-10-11: Review started
- 2026-10-11: Decided by Scott Schlesier (review round 1): the stylesheet's and favicon's `Last-Modified` is the running binary's modification time ("use time from the binary"), replacing the process start time. Assumed: if `os.Executable` or its stat fails, fall back to the current time.
- 2026-10-11: Triage round 1. Fixed: gofmt slip in `server/server.go`; CSS and favicon embedded as `[]byte`; tests compare against `server/template/` read from disk rather than the embedded variables; Pragma and Expires asserted; live reload script asserted present when on and absent when off; `Last-Modified` and 304 asserted. Dismissed: a decoy `template/` beside the executable (nothing in `server/` reads beside the executable; `os.Executable` is used only to stat the binary's mtime); the live reload script being injected when the watcher fails to start (predates this PR, and Boundaries forbid other HTML changes; worth its own spec); inlining `tmpl :=` (style only).
- 2026-10-11: Triage round 2. Fixed: `templateFS` embeds only `template/*.html` (the stylesheet and favicon were stored twice); the decoy test also requests `/favicon.ico`; new `TestIgnoresTemplateDirBesideBinary` puts decoys beside the test binary (this reverses round 1's dismissal: the test is cheap, and a read beside the binary added back would otherwise pass); the mtime test skips where the test binary can't be modified, and leaves its access time alone. Dismissed as equivalent: dropping the explicit Content-Type on the stylesheet or favicon, or renaming the `ServeContent` name argument (`ServeContent` derives the same type from the extension, and the explicit header wins).
