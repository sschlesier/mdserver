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

## Verification

- `go test ./...`
- `go test -race -count=5 -run Stop ./server`

## Design

- Guard `Server.Stop` with a `sync.Once` field on `Server`: the first call stops live reload,
  and later calls do nothing. `LiveReload.Stop` and `main` are unchanged.
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
