---
title: Add the missing tests for the missing-root shutdown
id: apt-rad
type: chore
priority: 2
depends-on: []
approved: "Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run"
status: in-review
---

The missing-root shutdown from PR #3 has tests for each behavior its spec promised, so a
regression fails CI.

Context: the PR #3 retro review's mutation testing found gaps. These changes to
server/server.go all passed the suite: dropping the folder name or `html.EscapeString` from
the 410 page; treating every stat error as "missing"; a regular file at the root path not
counting as missing; dropping `Connection: close`; running `Shutdown` synchronously (which
delays the 410 by the full timeout); and `Start` not waiting for shutdown to finish.

Out of scope:

- Any change to non-test code. If a test shows a real bug, stop and ask.
- The Rename path and the timeout, which have their own specs
  (`shut-down-when-root-renamed`, `root-missing-shutdown-timeout`).

## Acceptance criteria

- [x] A test serves a root whose name contains `<` and `&`, removes it, and checks that the
      410 body contains the HTML-escaped absolute path.
- [x] A test checks that the 410 response arrives within 1s of the request and has
      `Connection: close`.
- [x] A test makes the root's parent unsearchable (`chmod 000`), checks that a request gets a
      non-410 response and the server keeps running, then restores it. It's skipped when
      running as root and on Windows.
- [x] A test replaces the root with a regular file and checks for 410 and `ErrRootMissing`.
- [x] A test holds a request in flight, removes the root, releases the request within the
      shutdown timeout, and checks that `Start` doesn't return before that request finishes.
- [x] For each change listed in Context, at least one new test fails when that change is
      applied by hand (record which one in the Log).

## Verification

- `go test ./...`
- `go test -race -count=5 -run Root ./server`
- CI passes on ubuntu-latest.

## Design

- Order: last of the shutdown specs (`shut-down-when-root-renamed` →
  `root-missing-shutdown-timeout` → `guard-server-stop-against-double-call` → this one), so the
  tests are written against the final behavior.
- New tests go in `server/rootmissing_test.go`, reusing its helpers.
- A "request in flight" means a handler blocked on a channel the test controls; add it to a
  test-only mux, or use a markdown request with a slow reader. The implementer chooses.

## Log
- 2026-09-30: Approved: Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run
- 2026-10-03: Started on branch add-root-missing-shutdown-tests
- 2026-10-03: Implemented in server/rootmissing_test.go; no non-test code changed. Each Context mutation applied by hand to server/server.go and caught:
  - drop folder name from the 410 page: TestMissingRootNoticeEscapesRootPath
  - drop html.EscapeString: TestMissingRootNoticeEscapesRootPath
  - every stat error treated as missing: TestUnreadableRootParentIsNotTreatedAsMissing
  - regular file at root not missing: TestRootReplacedByFileIsMissing
  - drop Connection: close: TestMissingRootNoticeIsPromptAndClosesConnection
  - synchronous Shutdown: TestMissingRootNoticeIsPromptAndClosesConnection (and four others)
  - Start not waiting for shutdown: TestStartWaitsForInFlightRequestOnRootMissing
- 2026-10-03: The Go client strips the Connection header and reports it as resp.Close, so that test checks resp.Close.
- 2026-10-03: Review started
