---
title: Serve only Markdown and images from the root
id: ava-net
type: bug
priority: 2
depends-on: [era-sib]
approved: "Scott Schlesier, 2026-10-10: approved after cold read and SVG header decision. Cold read: pass"
status: in-review
---

mdserver serves only Markdown (rendered) and image files from the served folder, and any other
file gets 404. `/assets/...` URLs reach the folder's own `assets/` directory like any other
folder.

Context: from the review profile (confirmed 2026-09-30): "Only Markdown (.md, rendered) and
image files (.png .jpg .jpeg .gif .svg .webp) under the root are served; any other extension
gets 404." Today `handleStaticFile` (server/server.go) and `/assets/<path>` serve any file
under the root. Known types get their MIME type (`getContentType`), and anything else goes out
as `application/octet-stream`. So `.html`, `.js`, `.json`, `.go` and `.env.local` files are all
downloadable. `handleAssets` (server/handlers.go) also strips the `/assets/` prefix before
looking under the root, so `/assets/logo.png` serves `root/logo.png`, and a docs folder's own
`assets/logo.png` is reachable only at `/assets/assets/logo.png` (checked 2026-09-30). A
relative link `![](assets/logo.png)` from a root-level page gets the wrong file or a 404.
It depends on `hide-dotfiles-at-any-depth` because both change the same checks.

Out of scope:

- mdserver's own files (`/assets/style.css`, `/favicon.ico`, `/favicon.svg`, `/livereload`,
  `/settings`), which don't come from the root. `/assets/style.css` keeps shadowing a
  root `assets/style.css`; `move-mdserver-files-off-assets` removes that collision.
- Directory listings, which already show only directories and `.md` files.
- `--render` mode.

## Acceptance criteria

- [ ] Files ending in `.md` in any case (`.md`, `.MD`, `.Md`) are rendered, including
      extensionless URLs that resolve to `.md`. The directory listing's links to them open.
- [ ] `.png .jpg .jpeg .gif .svg .webp` files under the root are served with their image MIME
      type. The extension match ignores case (`.PNG` works).
- [ ] `/assets/<path>` resolves to `root/assets/<path>` under the same rules as any other URL:
      `/assets/logo.png` serves `root/assets/logo.png`, `/assets/page.md` renders
      `root/assets/page.md`, and `/assets/logo.png` no longer serves `root/logo.png`.
- [ ] Any other file under the root gets 404: for
      example `.html`, `.htm`, `.css`, `.js`, `.json`, `.txt`, `.pdf`, `.go`, and files with
      no extension.
- [ ] mdserver's own assets still load: a rendered page gets its stylesheet, favicon and live
      reload.
- [ ] Every image served from the root carries
      `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'` and
      `X-Content-Type-Options: nosniff`, so a script inside an SVG opened directly doesn't run.
      Rendered pages, directory listings and mdserver's own files don't get these headers.
- [ ] A table-driven test covers the allowed and blocked extensions above, and checks the
      image headers.
- [ ] README.md no longer says CSS and JS are served, and CHANGES.md has an unreleased entry
      for the `/assets/` remap, the 404 for other file types, and the image headers.

## Verification

- `go vet ./...` and `go test -race ./...` (as CI runs it)
- Manual: serve a folder with `a.md` linking `img.png`, plus `x.html` and `data.json`. A
  screenshot of `/a.md` shows the image. `curl -i` on `/x.html` and `/data.json` gives 404,
  and on `/img.png` gives 200 `image/png` with the CSP and nosniff headers.

## Design

- **Public behavior change:** `/assets/<path>` now means `root/assets/<path>` (a bug fix for
  folders with an `assets/` directory; anything relying on `/assets/X` → `root/X` breaks).
  A Markdown file linking a local `.html`, `.pdf` or `.txt` file
  will get a 404. That's intended, per the profile.
- One allowlist of extensions → MIME types replaces `getContentType`'s file branches. The
  HTML/CSS/JS/JSON entries go.
- `.svg` is served as `image/svg+xml`, as today.
- Markdown matching ignores case: any name whose extension lowercases to `.md`.
- `/assets/style.css` is the one exception under `/assets/`: it stays mdserver's stylesheet.
  Everything else under `/assets/` goes through the normal root handler (for example, route
  `/assets/` to `handleRequest` after the `style.css` check).
- `.ico` leaves the allowlist; mdserver's own favicon has its own handler.
- An extensionless URL tries only `<path>.md`, as today, not `.MD` or `.Md`. On a
  case-sensitive filesystem `/README` doesn't find `README.MD`; the directory listing links the
  real name, so it still opens.
- The order of checks stays: hidden path → 404, outside the root → 403, missing → 404, then
  the allowlist → 404.
- `extractTitle` and `createBreadcrumbs` strip `.md` in any case from display names.
- `server/dotpaths_test.go` expects `/assets/docs/pic.png` to serve `root/docs/pic.png`. That
  encodes the old prefix-stripping, so the test changes to a fixture under `root/assets/`.
  Its `/assets/...` dotfile cases stay 404.
- Image headers: the CSP stops scripts in an SVG opened as a top-level page, which could
  otherwise POST to `/settings/shutdown` on mdserver's origin. `<img>` embedding is
  unaffected, because SVG in `<img>` never runs scripts.

## Steps

1. server/server.go: replace `getContentType` with an image allowlist (extension → MIME,
   lowercased lookup, returns ok). `handleStaticFile` 404s anything off it and sets the CSP
   and nosniff headers on what it serves.
2. server/server.go `handleRequest`: match `.md` in any case.
3. server/handlers.go `handleAssets`: keep the `/assets/style.css` exception; send everything
   else to `handleRequest` unchanged.
4. Display names: case-insensitive `.md` stripping in `extractTitle` and `createBreadcrumbs`.
5. Tests: a table-driven allowlist test (extensions, MIME types, headers, `/assets/` cases,
   a listing link to an `.MD` file); update `server/dotpaths_test.go`.
6. README.md and CHANGES.md.

## Log

- 2026-09-30: Cold read: blocked on `.MD` case and `/assets/*.md`. Answered (Scott Schlesier):
  Markdown matching ignores case; `/assets/` is treated as any other folder, and the style.css
  collision gets its own ticket.
- 2026-10-10: Cold read: pass, no blocking questions. Folded in its assumptions and spec
  issues (extensionless lookup, check order, dotpaths test, README/CHANGES). SVG scripts
  (Scott Schlesier): add CSP and nosniff headers to served images.
- 2026-10-10: Approved: Scott Schlesier, 2026-10-10: approved after cold read and SVG header decision. Cold read: pass
- 2026-10-10: Started on branch serve-only-markdown-and-images
- 2026-10-10: Implemented. `go vet ./...` and `go test -race ./...` pass. Step 3 was committed
  before Step 1 so each commit builds (Step 1 removes `getContentType`, which `handleAssets`
  called); the `dotpaths_test.go` fixture change went with Step 3. Manual check: `curl -i` gives
  404 for `/x.html` and `/data.json`, and 200 `image/png` with CSP and nosniff for `/img.png`;
  `/a.md` renders `<img src="img.png">`. The screenshot was not taken: headless Chrome was
  blocked by the worktree guard. 404 responses carry `X-Content-Type-Options: nosniff` because
  Go's `http.NotFound` sets it; the test checks for the image headers' absence only on 200s.
- 2026-10-10: Review started
