---
title: Shut down when the served folder is renamed, even if something replaces it
type: bug
priority: 2
approved: "Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run"
status: done
---

Moving or deleting the served folder always shuts mdserver down, even if another folder
appears at the same path right away.

Context: found in the PR #3 retro review. With live reload on, a Remove or Rename event for the
root calls `onRootRemoved` (server/livereload.go:167), which only shuts down if `rootExists()`
is false (server/server.go, NewServer). After a quick swap (`mv root root.old && mv root.new
root`) the path exists again, so the server keeps running, but the watcher is still on the old
folder (now `root.old`). Live reload then silently stops working for the new folder. That breaks
the profile's rule that a saved file shows in open tabs within about a second. Decided in the
review (Scott Schlesier, 2026-09-30): moving the root is enough by itself to mean "root no
longer exists".

Out of scope:

- Detecting a move of a parent of the root. Waiting for the next request is fine (decided
  2026-09-30).
- Re-attaching the watcher to a replacement folder.
- The request path (`requireRoot`), which keeps using `rootExists()`.

## Steps to reproduce

1. `mkdir -p /tmp/r/root /tmp/r/new && echo hi > /tmp/r/root/a.md`
2. `go run . --dir /tmp/r/root --no-open`
3. `mv /tmp/r/root /tmp/r/root.old && mv /tmp/r/new /tmp/r/root`
4. Expected: the log shows `Directory /tmp/r/root no longer exists; shutting down` and the
   process exits 0. Actual: it keeps running, and live reload no longer fires for `/tmp/r/root`.

## Acceptance criteria

- [ ] With live reload on, a Remove or Rename event for the root shuts the server down, and
      `Start` returns `ErrRootMissing`, even if the path exists again when the event is handled.
- [ ] A test renames the root (`os.Rename`) and expects `ErrRootMissing`, with no request.
- [ ] A test renames the root and at once creates a new directory at the same path, and expects
      `ErrRootMissing`.
- [ ] Events on paths other than the root never shut the server down (existing live reload
      tests still pass).

## Verification

- `go test ./...`
- `go test -race -count=5 -run Root ./server`
- Manual: the Steps to reproduce above now give the expected result.

## Design

- The watcher callback calls `shutdownRootMissing` directly, with no `rootExists()` check.
- Remove and Rename are treated the same.
- The request path is unchanged: a request still shuts down only when `rootExists()` is false.

## Log
- 2026-09-30: Approved: Scott Schlesier, 2026-09-30: approved after review of PR #3 findings. Cold read: not run
- 2026-09-30: Started.
- 2026-09-30: Implemented in PR #4 (branch shut-down-when-root-renamed). Swap test failed 3/3 before the fix. go test ./... and -race -count=5 -run Root pass; manual repro on macOS exits 0 with the expected log line.
- 2026-09-30: Merged in PR #4 without a pr-review, and without a repo copy of this spec.
  Restored here from the spec store's history. Criteria are left unticked because no
  review verified them.
