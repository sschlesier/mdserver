---
title: Don't panic when the server is stopped twice
type: bug
priority: 3
depends-on: [root-missing-shutdown-timeout]
approved: "Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run"
status: in-review
---

Pressing Ctrl-C while mdserver is shutting down because its folder disappeared exits cleanly instead of panicking.

Context: found in the PR #3 retro review (unproven risk, accepted for that PR). When the root goes
missing, `main` calls `srv.Stop()` (main.go:155). The SIGINT/SIGTERM goroutine also calls
`srv.Stop()` (main.go:137-138). `Server.Stop` calls `LiveReload.Stop`, which does
`close(lr.stopChan)` with no guard. So a signal that lands in that window closes a closed
channel: the process panics and exits non-zero with a stack trace instead of 0. The window is a
few milliseconds.

## Steps to reproduce

1. Call `srv.Stop()` twice on a server with live reload on (a test can do this directly; the
   real window is too small to hit by hand).
2. Expected: the second call does nothing. Actual: `panic: close of closed channel`.

## Acceptance criteria

- [ ] Calling `Server.Stop()` twice, sequentially or concurrently, doesn't panic.
- [ ] A test calls `Stop()` concurrently from two goroutines with live reload on, and passes
      under `-race`.
- [ ] Exit codes don't change: 0 on a missing root, 0 on SIGINT/SIGTERM.
- [ ] (Added in review round 1.) A live-reload client that stops reading can't block `Stop`:
      with such a client connected, `Stop` returns within the write deadline plus 1s.

## Verification

- `go test ./...`
- `go test -race -count=5 -run Stop ./server`

## Design

- Guard `Server.Stop` with a `sync.Once` field on `Server`: the first call stops live reload,
  and later calls do nothing. `LiveReload.Stop` and `main` are unchanged.
- (Added in review round 1.) Each live-reload WebSocket write gets a 2s write deadline
  (matching `rootMissingShutdownTimeout`), so a client that stops reading fails its write,
  is dropped by the existing error path, and releases `clientsMu`. The deadline is a
  package-level var so a test can lower it.
- Order: after `root-missing-shutdown-timeout`, which also changes the shutdown path, so this
  one is written against its final shape.

## Log

- 2026-09-30: Needs clarification: worth fixing? If so: a `sync.Once` in `Server.Stop`, a
  `sync.Once` in `LiveReload.Stop`, or restructure `main` so only one path calls `Stop`.
- 2026-09-30: Answered (Scott Schlesier): worth fixing; `sync.Once` in `Server.Stop`.
- 2026-09-30: Approved: Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run
- 2026-09-30: Started on branch guard-server-stop-double-call.
- 2026-09-30: Implemented in commit 473efb8. Sequential double-Stop test panicked
  (`close of closed channel`) before the fix.
- 2026-09-30: Deviation: the spec was copied into the repo after implementation, at review
  time, not as the branch's first commit (Start step 3 was skipped).
- 2026-09-30: Review started.
- 2026-09-30: Pass 1 round 1: pass. Finding fixed: dropping `s.liveReload.Stop()` survived
  mutation; added an assertion that live reload is stopped.
- 2026-09-30: Dismissed: data race at server/livereload.go:197 (`len(lr.clients)` read
  outside `clientsMu`) fails `go test -race ./server` in TestLiveReloadIntegration. It
  predates this PR, which doesn't touch livereload.go; it needs its own ticket.
- 2026-09-30: Dismissed: Stop tests live in rootmissing_test.go rather than their own file.
  Cosmetic; that file already holds the shutdown-path tests.
- 2026-09-30: Pass 1 round 2: pass. 2/2 revert, 8 mutants, none survived.
- 2026-09-30: Dismissed: CI (.github/workflows/ci.yml:26) runs tests without `-race`, so a
  racy Stop guard would pass CI. Out of scope here; `-race` can't be added until the
  livereload.go:197 race is fixed, so it belongs with that ticket.
- 2026-09-30: Unproven risk from round 2: a stuck live-reload client can block the first
  Stop (WriteMessage under clientsMu.RLock with no write deadline), and Ctrl-C then waits
  behind it instead of panicking out. Sent to pass 2.
- 2026-09-30: Pass 2: the stuck-Stop risk is FOR PERSON (nothing in the spec or profile
  settles it). No broken "Valid while" conditions.
- 2026-09-30: Needs fixes (round 1): 1. A stuck live-reload client must not block `Stop`;
  Ctrl-C would otherwise wait behind it until SIGKILL.
- 2026-09-30: Spec change approved (Scott Schlesier): added criterion 4 and a Design bullet
  for a 2s per-write WebSocket deadline in livereload.go.
- 2026-09-30: Fixed 1.1: 2s per-write deadline in broadcastMessages.
  TestStopWithStuckLiveReloadClient blocked past 3s before the fix; it now returns in
  about 2.2s total.
- 2026-09-30: Scope change approved (Scott Schlesier): the new test exposed the earlier
  dismissed livereload.go race (`len(lr.clients)` outside clientsMu), failing this spec's
  `-race -run Stop` check. Fixed here as review item 1.2. `go test -race ./...` passes.
