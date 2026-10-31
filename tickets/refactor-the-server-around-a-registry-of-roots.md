---
agent: "STOP: run `spx help --skill` and follow it before any work on this spec. Use spx for every store step; never edit the store by hand."
title: Refactor the server around a registry of roots
type: task
priority: 2
depends-on: []
parent:
approved: "Scott Schlesier, 2026-10-31: Spec 1 of 3 (root registry refactor, no behaviour change): Config.RootDir kept, test-only two-root constructor, per-root LiveReload; response-diff verification. Cold read: pass"
status: in-review
---

The server resolves files and runs live reload through a per-root object held in a registry, so a later change can add more roots; mdserver's behavior with one root is unchanged for users.

Context: this is spec 1 of 3 toward serving several roots from one port (spec 2: multi-root mode from a static config; spec 3: live config reload). Today `Server` reads `s.config.RootDir` in about 20 places and owns a single `*LiveReload`. `LiveReload` broadcasts a bare `"reload"` to every connected client on any `.md` change, so two roots sharing one instance would reload each other's tabs. The two earlier attempts at this (`serve-roots-under-name-prefix`, `attach-root-to-running-instance`) were dropped, so this spec does only the internal seam and nothing user-visible.

Out of scope:

- Any new flag, config file, URL prefix, landing page or renderer change (specs 2 and 3).
- Changing `server.Config`, `main.go`, templates or any response byte.
- Per-root handling of a missing root directory. With the single root, the existing whole-server shutdown stays exactly as is.
- Choosing a root from a request. With one root the registry returns it; how a request picks among several is spec 2.

## Acceptance criteria

- [x] `go test ./...` passes, and every existing test keeps its assertions. Edits to existing tests are limited to renaming unexported identifiers the refactor moves.
- [x] Responses are unchanged: for a fixture tree, the status code, `Content-Type`, `Location` and body of each request in the Verification list are identical between a binary built from `main` before this change and one built from the branch.
- [x] `Server` no longer reads `Config.RootDir` outside the code that builds the registry; file resolution, directory listing, the file picker's file list, the settings page and remove-watch all go through a root object.
- [x] Each root owns its own `LiveReload` (watcher, client set and broadcast channel). `Server` holds no `LiveReload` of its own.
- [x] A test with two roots A and B: a client connected to A's `LiveReload` receives `"reload"` when a `.md` file in A changes, and a client connected to B's receives nothing, and the reverse.
- [x] A test with two roots A and B, using the root's resolve function directly, shows that for each of these the result never lies outside that root's directory (other than by following a symlink inside it): a path with `..` segments, an encoded `..`, a path naming a file in the other root, a dotfile, a file in a dot-directory, a non-Markdown/non-image extension; and a symlink inside the root pointing to a directory elsewhere is followed (accepted risk), which the test pins as current behavior. The same inputs are rejected for a single-root server through HTTP, as the existing tests already do.
- [x] `mdserver` with no flags, `--dir`, `--file`, `--live-reload=false` and `--render` behave as before (covered by the response diff and the existing tests).
- [x] CLAUDE.md's Project Structure is still accurate, or is updated if a file is added.

## Verification

- `go test ./...`, `go vet ./...` and `go build .` pass.
- Response diff (manual, expected: no difference):
  1. Before changing any code, build the pre-change binary from `main`'s head into the scratchpad: `go build -o <scratchpad>/mdserver-base .`. Keep the request-and-diff steps below as a script in the scratchpad so they can be rerun.
  2. Fixture: a directory with `a.md`, `sub/b.md` (linking to `a.md` and an image), `img.png`, `.hidden.md`, `.d/x.md`, `notes.txt`, an empty `empty/` directory, and a symlink inside the root to a sibling directory.
  3. Start each binary with `--dir <fixture> --port <p> --no-open`. For each of `/`, `/a.md`, `/sub/`, `/sub/b.md`, `/img.png`, `/empty/`, `/.hidden.md`, `/.d/x.md`, `/notes.txt`, `/nope.md`, `/../etc/hosts` (with `curl --path-as-is`), `/_mdserver/files`, `/settings`, `/livereload` (plain GET), record `curl -s -D- -o body` output; diff the pair after replacing the port number. Expected: identical.
  4. Open a page in a browser, edit its `.md` in another terminal, and confirm the tab reloads within about a second.
  5. Delete the served directory while the server runs: the "Folder missing" page appears and the process exits, as before.

## Design

Flags: none. Internal only. `server.Config` and the CLI are unchanged, so this is not a public API change.

Decisions:

- **`type root`** (unexported) holds the root's absolute directory and its `*LiveReload`. `Server` holds `roots []*root`, built from `Config.RootDir` in `NewServer`, so it has exactly one entry in production. The slice is not guarded by a mutex yet; spec 3 adds that when roots become dynamic.
- **`root.resolve(urlPath)`** carries the logic now in `handleRequest` (join, hidden-path check, directory check, `.md`-or-extensionless handling, static image check). It returns the filesystem path, a kind (page, directory, image, not found) and the status, so `handleRequest` still produces the same 301, 403 and 404 responses. The two-root test asserts "not ok or not inside the root", not exact statuses.
- **`/settings` ordering:** watched directories are listed in map order today. If `/settings` differs between two runs of the same binary, sort or retry that one request in the diff; that is baseline flakiness, not a regression. Don't change the page to make the diff pass.
- **Handlers take the root explicitly** (`s.handleIndex(w, r, root, ...)`, `root.resolve(urlPath)`, `root.relPath`), found through one `s.rootFor(r)` that returns the only root. Spec 2 replaces the body of `rootFor` with prefix routing.
- **`LiveReload` already has the right shape** (it takes `rootDir` and owns its watcher, clients and broadcast), so it moves from `Server` onto `root` with its behavior unchanged. The `/livereload` route is registered from the sole root's instance, as today.
- **Tests build more than one root through an unexported constructor** (for example `newServerWithRoots`) that production code does not use. This is the only way to test per-root isolation before spec 2 exists, and spec 2 will use it for real.
- **Missing root:** `onRootRemoved` and `requireRoot` keep their whole-server shutdown, which is correct for one root. They are written against `roots[0]` explicitly so spec 3 can see what to change.
- **Symlinks** stay followed, dotfiles and extensions stay filtered, exactly as today. The invariant tests lock that in at the resolve level, since that is the seam spec 2 builds on.

## Steps

1. Capture the baseline binary and response set (Verification steps 1-3) before changing code.
2. Add `type root` and `Server.roots`; build it in `NewServer`; add `rootFor`.
3. Move `liveReload` from `Server` to `root`; update `NewServer`, `Stop`, the `/livereload` route and the handlers in `handlers.go` that call `EnsureWatching`, `WatchedDirs` and `RemoveWatch`.
4. Replace each `s.config.RootDir` use in `server.go`, `handlers.go` and `files.go` with the root passed in or found with `rootFor`.
5. Add `newServerWithRoots` and the two-root tests (reload isolation, resolve invariants).
6. Run the full diff from Verification; rerun after any further change.

## Boundaries

Stop and ask if: keeping responses identical would need changing an existing test's assertion, a template, or any response byte; or the two-root tests need a non-test code path that changes behavior with one root.

Don't touch: `renderer/`, `main.go` flags, `server/template/`, `.github/`.

## Log

- 2026-10-31: Approved: Scott Schlesier, 2026-10-31: Spec 1 of 3 (root registry refactor, no behaviour change): Config.RootDir kept, test-only two-root constructor, per-root LiveReload; response-diff verification. Cold read: pass
- 2026-10-31: Started on branch refactor-root-registry
- 2026-10-31: Assumption: root.resolve rejects (403) a directory outside the root, a check the old inline code lacked because http.ServeMux cleans '..' before handlers run. Unreachable over HTTP so responses are unchanged; needed for the resolve-level criterion. Not a behavior change with one root.
- 2026-10-31: Verification run: go test/vet/build pass; response diff of 16 paths (base vs branch) identical, base-vs-base also identical; --file, --render, --live-reload=false and missing-root (410 + exit 0) match the baseline. Not done: step 4 (edit-and-reload in a real browser); live reload is covered by the existing integration tests and the new two-root test.
- 2026-10-31: Status: in-review
- 2026-10-31: Review round 1 triage: added tests for the 301 directory redirect, Stop on every root, EnsureWatching from handleIndex/handleMarkdown, and remove-watch (surviving mutants, all now killed). Dismissed: mutant '!HasSuffix(requestPath,/)' -> true is equivalent (requestPath and urlPath share the suffix); relPath 'outside returns original' only changes log text; code-review style notes on Abs/Rel repetition in contains/isValidPath/isHiddenPath, the newServerShell+finish constructor split, and resolve returning kind plus status are cosmetic; its nil-liveReload guess is wrong (handleSettings and handleRemoveWatch guard it).
