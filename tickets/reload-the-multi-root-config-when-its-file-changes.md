---
agent: "STOP: run `spx help --skill` and follow it before any work on this spec. Use spx for every store step; never edit the store by hand."
title: Reload the multi-root config when its file changes
type: feature
priority: 2
depends-on: [serve-several-roots-from-one-port-with-multi-root]
parent:
approved: "Scott Schlesier, 2026-10-31: Spec 3 of 3: live config reload, all-or-nothing, vanished root revived on request. Cold read: no behaviour gaps; verdict blocked only on unbuilt prerequisites; spec issues folded in"
status: in-review
---

When a user edits the multi-root config file while `mdserver --multi-root` is running, the roots served change within about a second without a restart, and a mistake in the edit never takes down roots that were working.

Context: spec 3 of 3. Spec 2 reads the config once at startup. This spec makes the root set dynamic. It relies on spec 1's per-root object (each root owns its watcher and sockets, so one can be added or removed on its own) and spec 2's parser and validator, which it reuses unchanged. The review profile's staleness target is "a saved file shows in open tabs within about a second"; the config follows the same target. There are no tickers: reloads are driven by file-system events, and revival of a vanished root is driven by a request. Several criteria below (the unavailable-root state, error messages, the roots page) are defined by spec 2, so they can only be built and verified after it lands.

Out of scope:

- Changing the port while running. A changed `port` line is ignored and reported as needing a restart.
- Any CLI that edits the config, and watching more than the one config file.
- Retargeting a symlinked config path after startup (if the link itself is replaced to point elsewhere, restart).
- Reloading in single-root mode, which has no config.

## Acceptance criteria

- [x] Adding a `<name> <path>` line makes `/<name>/` serve that directory and appear on `/` within 2 seconds of the save, with no restart. Tabs open on other roots are not disturbed (their live-reload sockets stay connected).
- [x] Removing a root's line makes `/<name>/…` return 404 within 2 seconds, closes that root's live-reload sockets, stops its file watchers, and drops it from `/`.
- [x] Changing a root's path serves the new directory under the same name within 2 seconds, and the old directory's watchers and sockets for that name are closed.
- [x] Roots whose name and resolved directory are unchanged keep their existing live-reload instance and open sockets across a reload (checked by identity or by a socket staying open).
- [x] Both save styles are detected: an in-place write, and write-temp-then-rename over the config. A burst of several writes within a few hundred milliseconds results in one reload.
- [x] If the edited file is invalid in any way spec 2 refuses at startup (including zero roots), the served roots are unchanged: no root is added, removed or changed. `/` shows an error banner with the `file:line: reason` message, the log has it once, and the banner clears after the next valid save.
- [x] If the config file is deleted, unreadable, or briefly missing during an atomic save, the last good roots stay served, `/` shows the error, and a later valid file at the same path is loaded.
- [x] A change to the `port` line changes nothing in the running server; `/` shows a notice "port change needs a restart" and the log says so once.
- [x] A reload that adds a root whose path fails spec 2's path checks (relative, refused footgun path, not a directory) is treated as an invalid edit and applies nothing, including other valid changes in the same file.
- [x] A request under the prefix of a root marked unavailable (its directory vanished) re-checks the directory; if it exists again the root is brought back (new watchers and sockets) and the request is served. No timer is involved.
- [x] Requests in flight while a reload happens complete without a 5xx, and `go test -race` reports no data race with concurrent requests and reloads.

## Verification

- `go test -race ./...`, `go vet ./...` and `go build .` pass.
- Tests use a temp config file and temp root directories, and wait for effects with a 2 second deadline (poll the condition, never a fixed sleep). They cover each criterion above, with an in-place write and a rename-over-the-file save, a burst of writes (assert the reload count), each class of invalid edit (reuse spec 2's invalid-input table), deletion and recreation of the file, a socket opened before a reload that must stay open, a removed root's socket that must close, and a hammer of concurrent requests during repeated reloads.
- Manual (continuing spec 2's fixture, `/tmp/mr/roots` with roots `a` and `b`):
  1. Start `go run . --multi-root /tmp/mr/roots`, open `/`, `/a/x.md` and `/b/x.md` in three tabs.
  2. Append `c /tmp/mr/c` after `mkdir /tmp/mr/c && echo '# C' > /tmp/mr/c/x.md`. Within a second, refreshing `/` lists `c`; `/c/x.md` renders; the other tabs still live-reload.
  3. Edit the file in an editor that saves by rename (for example vim with default settings) and in one that writes in place; both are picked up.
  4. Change a line to `d ../x` (a relative path). `/` shows the same `file:line: reason` message spec 2 gives for that mistake at startup; `/a/` and `/b/` still work. Fix it; the banner disappears.
  5. Delete the `b` line: `/b/x.md` is 404 and its tab stops reloading.
  6. Change `port 8099` to `port 8100`: notice appears, nothing else changes.
  7. `rm -r /tmp/mr/a`, load `/a/` (410), `mkdir /tmp/mr/a` with a file, reload `/a/`: it serves again.

## Design

Flags: **config change** only in the sense that the user's config file now has live meaning; no schema change, no new flag, no public API.

Decisions:

- **Watch the config file's directory, not the file**, with fsnotify (already a dependency), and react to events whose name is the config file's base name. Editors that save by rename replace the inode, which silently ends a watch placed on the file itself. The path is resolved with `EvalSymlinks` at startup, and the directory of the resolved path is watched, so a config that is a symlink into a dotfile repo has its target edited and noticed.
- **Debounce about 100 ms** after the last event, then read and parse the whole file once.
- **All or nothing.** Parse and validate with spec 2's validator against the new content. On any error, apply nothing and record the error; on success, apply the diff atomically.
- **Diff by name:** same name and same resolved directory = unchanged (keep the root object); name gone = remove; new name = add; same name with a different resolved directory = remove then add.
- **Atomic swap:** `Server` holds the roots behind an `RWMutex` (or an atomically replaced immutable snapshot). Each request takes a snapshot at the start, so a request in flight on a removed root finishes normally. Stopping a removed root's `LiveReload` happens after the swap.
- **Errors are state, not exits.** The reload error (and the port notice) are kept on `Server`, shown as a banner on `/` and logged once per distinct message. A valid reload clears them. Startup errors stay fatal (spec 2).
- **Config path missing or unreadable during reload** is an error like any other; the watcher stays on the directory so the file coming back is noticed.
- **Zero roots is invalid** on reload, as at startup. A truncated file mid-save therefore cannot empty the server.
- **Port:** the value parsed at startup is kept; later changes only set the notice.
- **Vanished roots revive on request.** The per-request existence check from spec 2 is the trigger: if a root marked unavailable has its directory again, create a fresh root object (watchers, sockets) and swap it in. Unavailable roots are not retried on any timer, matching the project's rule against polling for nice-to-have detection.
- **Open tabs of a removed or changed root** are not navigated; their sockets close and they simply stop live-reloading, and a manual refresh shows the new state.

## Steps

1. `server`: make the root set swappable behind a mutex or snapshot; add add, remove and diff operations that start and stop a root's `LiveReload`; tests with `-race`.
2. `server`: add the config watcher (resolve symlinks, watch the directory, filter, debounce) feeding a `reload` function that parses with spec 2's validator and applies the diff.
3. `server`: add reload-error and port-notice state, the banner on `/`, and the once-only logging.
4. `server`: add request-driven revival of unavailable roots.
5. `main.go`: pass the resolved config path to the server; stop the watcher in `Stop`.
6. Tests per Verification; README and CHANGES.md notes.

## Boundaries

Stop and ask if: applying a diff would need a request to wait on a reload, or `-race` shows a lock-order problem that needs more than the root-set lock to fix; or the editor-compatibility tests show an event pattern the directory watch misses.

Don't touch: the config grammar and validation rules from spec 2 (reuse them as they are).

## Log

- 2026-10-31: Approved: Scott Schlesier, 2026-10-31: Spec 3 of 3: live config reload, all-or-nothing, vanished root revived on request. Cold read: no behaviour gaps; verdict blocked only on unbuilt prerequisites; spec issues folded in
- 2026-10-31: Started on branch reload-multi-root-config
- 2026-10-31: Status: in-progress
- 2026-10-31: Assumption: a changed port line is compared with the config's own port line at startup (0 when absent), not with --port; unreadable-file case shares the missing-file code path and is covered by the deletion test only
- 2026-10-31: Status: in-review: Implemented on reload-multi-root-config; go test -race, vet, build pass
- 2026-10-31: Review round 1 triage: added tests for stop-on-revive, picker revival, revive branches, unrelated-file filter and serialised reloads; dismissed WatchConfig/Errors-branch coverage as low value and the first-pass sameRootDir mutant as equivalent (unused fresh roots are never created for kept names)
