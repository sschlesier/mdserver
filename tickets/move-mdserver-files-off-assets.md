---
title: Move mdserver's own files off /assets/
id: hen-fur
type: bug
priority: 3
depends-on: [serve-only-markdown-and-images]
approved: "Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run"
status: in-progress
---

mdserver's own stylesheet moves to a URL prefix no served folder would use, so a folder's own
`assets/style.css` is reachable like any other file.

Context: after `serve-only-markdown-and-images`, `/assets/<path>` maps to `root/assets/<path>`,
except `/assets/style.css`, which is mdserver's stylesheet. It shadows a served folder's own
`assets/style.css`. (Today that only matters for images and Markdown: under the allowlist, a
root `.css` file isn't served anyway. The collision is on the URL namespace.) Decided in review
(Scott Schlesier, 2026-09-30): separate ticket.

Out of scope:

- `/favicon.ico`, `/favicon.svg`, `/livereload` and `/settings`. They could collide with a root
  `settings.md` (`/settings`) or a `livereload` file. Note any such collision in the Log, but
  don't move them.

## Acceptance criteria

- [ ] mdserver's stylesheet is served at `/_mdserver/style.css`, and every page it renders
      (Markdown page, directory index, settings, shutdown page, 410 page if styled) links there.
- [ ] `/assets/style.css` is no longer special: it follows the normal root rules.
- [ ] A test serves a folder with `assets/logo.png` and `assets/style.css`, and checks that
      `/assets/logo.png` serves the folder's file and that a rendered page still gets
      mdserver's stylesheet.

## Verification

- `go test ./...`
- Manual: `go run . --dir <folder with assets/logo.png> --no-open`; a page linking
  `assets/logo.png` shows the image, and the page is styled.

## Design

- Prefix: `/_mdserver/`. A served path whose first segment is `_mdserver` is shadowed, which is
  accepted: it's unlikely and documented here.
- Templates in `template/` and the inline HTML in server/handlers.go change their stylesheet
  link. Existing tests that check `href="/assets/style.css"` change with them.

## Log
- 2026-09-30: Approved: Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run
- 2026-10-11: Started on branch move-mdserver-files-off-assets
