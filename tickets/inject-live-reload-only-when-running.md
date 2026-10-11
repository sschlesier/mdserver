---
title: Inject the live reload script only when live reload is running
type: bug
priority: 3
depends-on: [embed-mdserver-resources]
parent:
approved: "Scott Schlesier, 2026-10-11: inject the live reload script only when s.liveReload != nil. Cold read: not run"
status: in-progress
---

When live reload is on but the file watcher fails to set up, pages no longer carry the live
reload client script, so open tabs stop retrying a `/livereload` endpoint that doesn't exist.

Context: `NewServer` parses the page templates with the live reload script whenever
`Config.EnableLiveReload` is set (`parseTemplates(config.EnableLiveReload)` in
`server/server.go`). If `NewLiveReload` or `LiveReload.Start` fails, it logs the error and
leaves `s.liveReload` nil, so `/livereload` is never registered. Every page still carries the
script, whose `onclose` handler reconnects every second, so each open tab requests a 404
WebSocket once a second for as long as it stays open. The settings page already gates its
watched-directories section on `s.liveReload != nil`, so it's consistent today; only the
script injection isn't. The bug predates `embed-mdserver-resources`. It was found in that
spec's review (PR #15) and left out because its Boundaries forbade other HTML changes.

Out of scope:

- Closing the fsnotify watcher when `Start` fails (it isn't closed today). Worth its own
  spec if it matters.
- Retrying live reload setup later, or telling the user in the page that live reload is off.
  The existing log line is the only notice.
- Changing the client script's reconnect behavior.

## Steps to reproduce

No flag or ordinary directory makes the watcher fail without also breaking the page: on
macOS, the root can't be watched only if it can't be opened, and then it can't be listed
either. Reproduce through the test seam in Design:

1. Make watcher creation fail, and start a server with `EnableLiveReload: true`.
2. GET `/` or any Markdown page.

Expected: no live reload script in the page, and no `/livereload` route.
Actual: the page contains the script (`new WebSocket(... '/livereload')`), and
`/livereload` returns 404.

## Acceptance criteria

- [x] With `EnableLiveReload: true` and watcher creation (`NewLiveReload`) failing, a
      directory index, a Markdown page and `/settings` contain no live reload script.
- [x] The same holds when `LiveReload.Start` fails.
- [x] With live reload running, all three pages still contain the script, and with
      `EnableLiveReload: false` none do (existing `TestLiveReloadScriptFollowsConfig`
      keeps passing).
- [x] The new tests fail when the fix is reverted.

## Verification

- `go test ./...`
- `go test ./server -run LiveReload -v`: the new failure-path tests pass.

## Design

- Decided: inject the script when `s.liveReload != nil`, the same condition the settings
  page and `/livereload` route already use. Parse the templates after the live reload setup
  in `NewServer`, not in the struct literal.
- Decided: test seam. Add an unexported package variable in `server/` that `NewServer` calls
  to create the watcher (defaulting to `NewLiveReload`), so a test can make it return an
  error. For the `Start` failure, have the seam return a `LiveReload` whose `Start` fails:
  for example, one with a `rootDir` that doesn't exist. Keep the seam unexported. No exported
  API changes.
- Flags: none.

## Steps

1. In `server/server.go`, move `parseTemplates` after the live reload block and pass
   `s.liveReload != nil`.
2. Add the watcher-creation seam.
3. Add tests for both failure paths in `server/embed_test.go`, next to
   `TestLiveReloadScriptFollowsConfig`.

## Boundaries

Stop and ask if: the fix needs a change to `Config`, a CLI flag, a served path, or the client
script itself.

## Log

- 2026-10-11: Drafted from the review of embed-mdserver-resources (PR #15).
- 2026-10-11: Approved: Scott Schlesier, 2026-10-11: inject the live reload script only when s.liveReload != nil. Cold read: not run
- 2026-10-11: Started on branch inject-live-reload-only-when-running
- 2026-10-11: Assumption: the Start-failure test builds the watcher with NewLiveReload and points its rootDir at a missing directory, as Design suggests. Revert check: with `parseTemplates(config.EnableLiveReload)` restored, both new subtests fail on all three pages.
