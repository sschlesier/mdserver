---
title: Never serve dotfiles or dot-directories at any depth
id: era-sib
type: bug
priority: 1
approved: "Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: pass"
status: in-review
---

No URL serves, renders or lists a file or directory under the root whose path has any segment
starting with `.`, such as `docs/.env` or `.git/`.

Context: found in the PR #3 retro review. It breaks the profile's rule: "Dotfiles and
dot-directories under the root are never served." `isValidPath` (server/server.go:263) checks
only the first character of the path relative to the root, so `/docs/.env` and
`/notes/.secret.md` are served. Directory requests (`/.git/`) go to `handleIndex` without any
path check, so a dot-directory's markdown files and subdirectories are listed. `/assets/<path>`
serves root files through the same check. Listings already hide dot entries.

Out of scope:

- The file-type allowlist (`serve-only-markdown-and-images`).
- Symlink handling: symlinks are followed, as an accepted risk.

## Steps to reproduce

1. `mkdir -p /tmp/d/docs && echo SECRET > /tmp/d/docs/.env && mkdir /tmp/d/.hidden && echo x > /tmp/d/.hidden/a.md`
2. `go run . --dir /tmp/d --no-open --port 8123`
3. `curl -s localhost:8123/docs/.env` → expected 404; actual `SECRET`.
4. `curl -s localhost:8123/.hidden/` → expected 404; actual a listing showing `a.md`.

## Acceptance criteria

- [x] A request whose path has any segment starting with `.` (other than the URL-cleaned `.`
      and `..`) gets 404, for markdown, static files, `/assets/` and directories. Cases:
      `/docs/.env`, `/notes/.secret.md`, `/.hidden/`, `/.hidden` (404, not a redirect to
      `/.hidden/`), `/.hidden/a.md`, `/.hidden/a`, `/assets/docs/.env`, `/assets/.env`, and
      the same paths URL-encoded (`%2e`).
- [x] Top-level dotfiles (`/.env`) also get 404; today they get 403.
- [x] Files and directories without a dot segment are served as before (existing tests pass).
- [x] A table-driven test covers every case above.

## Verification

- `go test ./...`
- Manual: the Steps to reproduce give 404.

## Design

- Status for a blocked dot path: 404, not 403, so the server doesn't reveal that the file
  exists.
- One helper decides "contains a dot segment", on the path relative to the root split by
  separator. `isValidPath` uses it, and `handleIndex` and the directory branch of
  `handleRequest` get the same check.
- `.well-known` is not special-cased.
- The dot check on a directory runs before the trailing-slash redirect.
- Paths that resolve outside the root keep their current 403. Only dot paths change to 404.
- The check runs on the path relative to the root, so a root whose own name starts with `.`
  (`--dir ~/.notes`) still works.

## Log

- 2026-09-30: Cold read: pass. Folded in: `/.hidden` without a slash, traversal keeps 403,
  root named with a leading dot.
- 2026-09-30: Approved: Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: pass
- 2026-10-03: Started on branch hide-dotfiles-at-any-depth
- 2026-10-03: Implemented. `go test ./...` passes; with the fix reverted the new table test
  fails on all 19 blocked cases. The manual curl repro was not run (the command was denied);
  the httptest table drives the same requests through the real handler.
- 2026-10-03: Review started (PR #9)
- 2026-10-03: Review round 1 triage. Fixed: no test covered the outside-root 403 (mutants M11 and
  M9 survived); added `TestOutsideRootPathsStayForbidden`, which kills M11. Also added `%2f` and
  `/.hidden/.` cases (the latter is a mux 307 to `/.hidden/`, which is 404). Dismissed as
  equivalent mutants: dropping the hidden check in `handleIndex`, `handleStaticFile` or
  `isValidPath` (every caller is already guarded in `handleRequest`; the copies are defence in
  depth per Design), skipping `..` in `hasDotSegment` (`filepath.Rel` never yields `..` mid-path),
  and the `filepath.Abs` error branch (unreachable). `claude -p "/code-review 9"`: no findings.
