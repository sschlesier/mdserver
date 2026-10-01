---
title: Wait 2s for in-flight requests on a missing-root shutdown, and log a timeout
type: task
priority: 3
depends-on: [shut-down-when-root-renamed]
approved: "Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run"
status: done
---

When mdserver shuts down because its folder is gone, it gives in-flight requests up to 2s
instead of 5s, and it logs when that deadline cuts requests off.

Context: decided in the PR #3 retro review (Scott Schlesier, 2026-09-30). Today
`shutdownRootMissing` (server/server.go) calls `httpServer.Shutdown` with a 5s timeout and
ignores the error.

## Acceptance criteria

- [ ] The graceful shutdown for a missing root waits at most 2s for in-flight requests.
- [ ] If the deadline passes before they finish, the log shows one line saying the shutdown
      timed out, and in-flight requests are cut off. `Start` still returns `ErrRootMissing`,
      and the process still exits 0.
- [ ] If requests finish in time, no timeout line is logged.
- [ ] A test holds a request open past the deadline, removes the root, and checks that `Start`
      returns `ErrRootMissing` within about 2s (under 3s) and that the timeout line is logged.

## Verification

- `go test ./...`
- `go test -race -count=3 -run Root ./server`

## Design

- The timeout is a package-level value (e.g. `rootMissingShutdownTimeout = 2 * time.Second`),
  so a test can lower it. It isn't a CLI flag.
- Log text: `Shutdown timed out after 2s; closing remaining connections`. After the timeout,
  call `httpServer.Close()` so nothing lingers.
- Order: after `shut-down-when-root-renamed`; both change the missing-root shutdown path.

## Log
- 2026-09-30: Approved: Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run
- 2026-09-30: Started.
- 2026-09-30: Implemented in PR #5 (branch root-missing-shutdown-timeout). Stuck-request test failed before the fix; Close() mutation caught. go test ./... and -race -count=3 -run Root pass.
- 2026-09-30: Merged in PR #5 without a pr-review, and without a repo copy of this spec.
  Restored here from the spec store's history. Criteria are left unticked because no
  review verified them.
