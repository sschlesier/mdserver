---
agent: "STOP: run `spx help --skill` and follow it before any work on this spec. Use spx for every store step; never edit the store by hand."
title: Serve several roots from one port with --multi-root
type: feature
priority: 2
depends-on: [refactor-the-server-around-a-registry-of-roots, rewrite-root-relative-markdown-links-under-a-url-prefix]
parent:
approved: "Scott Schlesier, 2026-10-31: Spec 2 of 3: --multi-root with line-format config, zero roots invalid, footgun list incl. /private/var and /private/etc, accepted-risk rewording via PR. Cold read: no behaviour gaps; verdict blocked only on unbuilt prerequisites; spec issues folded in"
status: done
---

A user can run `mdserver --multi-root [config]` to serve several named root directories from one port, each at `/<name>/`, with the roots read once at startup from a plain-text config file; running mdserver without `--multi-root` is unchanged.

Context: spec 2 of 3 (it can be built only after its two prerequisites are implemented) (spec 1 is the root registry refactor; spec 3 reloads the config live). Today one instance serves one directory at `/`, so serving several means several processes on several ports. Single-root mode stays as the simple, predictable default: no flags means one root, localhost, no URL prefix. Multi-root is opt-in, so no config file ever affects a plain `mdserver` run. Two earlier attempts at multi-root were dropped (`serve-roots-under-name-prefix`: it prefixed single-root URLs too, and rewriting root-relative Markdown links felt like too much). This spec keeps single-root URLs unchanged. The link rewriting is a separate prerequisite spec (`rewrite-root-relative-markdown-links-under-a-url-prefix`) that this one calls with the root's prefix in multi-root mode only.

Out of scope:

- Changing the config while the server runs (spec 3). Here the file is read once.
- Any CLI to edit the config (`mdserver add`). The file is edited by hand.
- Environment-variable expansion in paths. Only a leading `~/` is expanded.
- Relative paths in the config, per-root options, and includes.
- Making `--host` refuse multi-root. It works as it does for single-root.
- Redirects from old single-root URLs, and rewriting raw HTML in Markdown (raw HTML is not passed through today).

## Acceptance criteria

- [x] Without `--multi-root`, behavior is unchanged: the existing tests pass unmodified and the single-root URLs, listings, breadcrumbs and settings page are as before.
- [x] `mdserver --multi-root` with no argument reads `$XDG_CONFIG_HOME/mdserver/roots` when `XDG_CONFIG_HOME` is set and the file exists there, else `~/.config/mdserver/roots`. `mdserver --multi-root <path>` reads that file. If the file does not exist, the process exits 1 with a message naming each path it looked at, and creates nothing.
- [x] Config grammar: blank lines and lines whose first non-space character is `#` are ignored. `port <n>` sets the port (1-65535). Any other line is `<name> <path>`: the name is the first whitespace-delimited token, the path is the rest of the line, trimmed, and may contain spaces. There are no inline comments and no quoting.
- [x] A name matches `[a-z0-9][a-z0-9_-]*` and is not reserved. Reserved: `port`, `settings`, `livereload`, `favicon.ico`, `favicon.svg`, `_mdserver`.
- [x] A path must start with `/` or `~/`; `~/` expands to the user's home directory. Nothing else is expanded: a path starting with `$`, `~user` or a relative path is refused.
- [x] A path must exist and be a directory. After resolving symlinks it must not be the filesystem root, the user's home directory, an ancestor of the home directory, or one of `/Users`, `/home`, `/etc`, `/var`, `/usr`, `/bin`, `/sbin`, `/opt`, `/private`, `/private/var`, `/private/etc`, `/System`, `/Library` (the directory itself; subdirectories are allowed).
- [x] Any invalid line (bad name, reserved or duplicate name, bad path, refused path, second `port` line, bad port, a line with a name but no path) or a file with zero roots makes the process exit 1 before listening, with a message `<file>:<line>: <reason>`.
- [x] The listening port is `--port` if given (a config `port` is then ignored), else the config's `port`, else an automatically chosen free port. `--host` behaves as today.
- [x] `--multi-root` together with `--dir`, `--file` or `--render` exits 2 with a usage message naming the conflict.
- [x] `/` shows a page listing every root, in config order, each linking to `/<name>/`. The browser opens at `/` on startup (unless `--no-open`), and startup prints one `Serving <name>: <abs path>` line per root and the URL.
- [x] `/<name>` redirects with 301 to `/<name>/`. `/<name>/` is that root's directory listing; `/<name>/sub/page.md` renders a page; `/<name>/img.png` serves an image. A first path segment that is neither a root name nor a reserved route returns 404.
- [x] Per root, every existing invariant holds: no request under `/<name>/` serves, renders or lists anything outside that root's directory (other than through a symlink inside it), including `..`, encoded `..` and a path naming another root's files; dot-segments are never served; only `.md` and the allowlisted image types are served; the server writes nothing.
- [x] In multi-root mode, a page rendered under `/<name>/` is rendered with the prefix `/<name>`, so a root-relative link or image in it (`[x](/sub/b.md)`, `![](/i.png)`) points to `/<name>/sub/b.md` and `/<name>/i.png`; relative and absolute URLs are unchanged. In single-root mode pages are rendered with no prefix, as today.
- [x] Directory-listing links and breadcrumbs in multi-root mode carry the prefix: breadcrumbs are home icon → `/`, then `<name>/`, then subdirectories, then the file; the `..` entry at a root's top level links to `/`.
- [x] Live reload is per root: with two roots A and B served and a tab open on each, saving a `.md` file in A reloads the tab on A and does not reload the tab on B, and the reverse.
- [x] The Alt-O file picker searches only the current page's root, and each result opens the right file under that root's prefix.
- [x] The settings page, in multi-root mode, groups watched directories under their root's name. Remove-watch removes only a directory of the named root, and a root's top-level watch cannot be removed. Shutdown stops the whole server.
- [x] If a root's directory disappears while the server runs, requests under its prefix return 410 with a "Folder missing" page naming it, its entry on `/` is marked unavailable, and the other roots keep serving. The server does not shut down in multi-root mode, even if every root becomes unavailable. The root stays unavailable until restart.
- [x] The README, `--help` text and CHANGES.md describe `--multi-root`, the config format and the default location; CLAUDE.md's Review profile invariants (the bullet list under `Invariants:` only, not the Purpose, Data or accepted-risk text) say "configured root" where they say "served root" (the accepted-risk text is not edited by this work; see Boundaries).

## Verification

- `go test -race ./...`, `go vet ./...` and `go build .` pass.
- Tests cover: the parser (each grammar and error case above, with the `file:line` message), default-path resolution with and without `XDG_CONFIG_HOME` (using temp directories and `t.Setenv`), the footgun list (including a symlink that resolves to home), flag conflicts, prefix routing and the 404/301 cases, the per-root invariants with two roots (extend spec 1's resolve tests to HTTP), that multi-root pages are rendered with their root's prefix and single-root pages without one (the destination forms themselves are tested in the renderer spec), per-root reload isolation over a real WebSocket, the picker's root scoping, settings grouping and scoped remove-watch, and the missing-root 410.
- Manual:
  1. `mkdir -p /tmp/mr/a/sub /tmp/mr/b && printf '# A\n\n[b](/sub/b.md) ![i](/i.png) [rel](sub/b.md)\n' > /tmp/mr/a/x.md && echo '# B' > /tmp/mr/a/sub/b.md && echo '# other' > /tmp/mr/b/x.md`
  2. Write `/tmp/mr/roots` containing `port 8099`, `a /tmp/mr/a`, `b /tmp/mr/b`.
  3. `go run . --multi-root /tmp/mr/roots`. The browser opens `http://localhost:8099/` listing `a` and `b`.
  4. Open `/a/x.md`: the `b` link goes to `/a/sub/b.md`, the image src is `/a/i.png`, the relative link still works. Open `/b/x.md` in a second tab; edit `/tmp/mr/a/x.md`: only the first tab reloads.
  5. `curl -s -o /dev/null -w '%{http_code}\n' --path-as-is http://localhost:8099/a/../b/x.md` prints `301` (the mux cleans the path and redirects to `/b/x.md`) or `404`, never `200`.
  6. Add a line `bad ../x` to the file and start again: exit 1, message `…/roots:4: path must start with / or ~/`.
  7. Delete `/tmp/mr/b` while running: `/b/` shows "Folder missing" (410), `/a/` still works.

## Design

Flags: **public API** (a new CLI flag, a new config file and its format, and a new URL scheme in the new mode) and **config change** (a new user config file; no deployment config). Additive, so a minor version bump. Single-root mode and its URLs do not change.

Decisions:

- **`--multi-root` is a bool flag; the optional config path is the first positional argument**, like the existing `[file]` positional in the usage line. (Go's `flag` package cannot take an optional value on a flag.) It is documented as `mdserver --multi-root [config]`.
- **Opt-in only.** No flag-less run reads a config file. With `--multi-root`, the roots come only from the config.
- **Config format** is the grammar in the criteria. The path is the rest of the line so paths with spaces need no quoting. `port` is a reserved name, not a root.
- **Default location:** if `XDG_CONFIG_HOME` is set and non-empty, `$XDG_CONFIG_HOME/mdserver/roots`; otherwise (or if that file is missing) `~/.config/mdserver/roots`. On macOS this deliberately uses the XDG-style path, not `~/Library`.
- **Startup validation is strict and fatal**, with `file:line` messages. Spec 3 keeps the same validator for reloads but handles failure differently.
- **Footgun list** is deliberately short and lives in one variable in the parser, so it is easy to edit. Serving only `.md` and image files and hiding dotfiles already limits what a mistaken root can expose; the list guards against the obvious mistakes.
- **`server.Config` gets `Roots []Root{Name, Dir string}`.** Exactly one of `RootDir` (single-root, unchanged) or `Roots` (multi-root) is set. `NewServer` builds the registry from whichever is set. Unprefixed routing stays the code path for `RootDir`; prefixed routing is selected by `Roots`.
- **Routing:** `rootFor(r)` becomes: single-root → the only root and the full URL path; multi-root → the root named by the first path segment and the remainder as the path inside it. Reserved routes (`/_mdserver/…`, `/settings…`, `/livereload`, `/favicon.*`) are matched before roots and keep their absolute paths.
- **Which root a script request belongs to:** pages tell client scripts their root (for example a `data-root` attribute on `<body>`, empty in single-root mode). The picker sends `root=<name>` to `/_mdserver/files`, and the live-reload client connects to `/livereload?root=<name>`. An unknown or missing root in multi-root mode is a 404; the parameter is ignored in single-root mode.
- **Renderer:** multi-root pages call `renderer.RenderMarkdownWithBase(md, "/"+name)` from the prerequisite spec; single-root pages keep calling `RenderMarkdown`. Cross-root links are written as full URLs. `--render` is untouched.
- **Missing root in multi-root mode** is per root (410 page, listed as unavailable), never a process shutdown. The existing whole-server shutdown stays for single-root mode only. A vanished root stays unavailable until restart; spec 3 revisits this.
- **Settings:** `/settings` stays one page and gains root headings; shutdown stays global (the existing accepted risk that the shutdown endpoint has no CSRF check is unchanged but now stops every root).
- **Overlapping roots and symlinks** are allowed and not special-cased.

## Steps

1. `server`: add the config parser and validator (a function from file contents plus home directory to `[]Root` and port, with `file:line` errors) and its tests. No wiring yet.
2. `server`: add default-path resolution and the footgun check, with tests.
3. `server`: add `Config.Roots`, prefix routing in `rootFor`, the `/` roots page, the 301, 404 and reserved-route handling; extend the per-root invariant tests to HTTP.
4. `server`: carry the prefix through listings, breadcrumbs, picker results, the `data-root` attribute and the picker and live-reload clients; scope `/livereload` and `/_mdserver/files` by `root`.
5. `server`: group the settings page by root and scope remove-watch; add the per-root missing-root 410 and its landing-page marker.
6. `main.go`: add `--multi-root`, flag-conflict checks, port precedence, startup lines and browser URL.
7. Docs: README, `--help`, CHANGES.md, CLAUDE.md Review profile invariants.

## Boundaries

Stop and ask if: a template, test or route outside those named here depends on `/`-rooted file URLs in a way the prefix design doesn't cover; or single-root behavior would have to change.

Don't touch: the accepted-risk entries and their "valid while" lines in CLAUDE.md. Instead, put suggested rewordings for multi-root in the PR description for Scott Schlesier to accept.

## Log

- 2026-10-31: Approved: Scott Schlesier, 2026-10-31: Spec 2 of 3: --multi-root with line-format config, zero roots invalid, footgun list incl. /private/var and /private/etc, accepted-risk rewording via PR. Cold read: no behaviour gaps; verdict blocked only on unbuilt prerequisites; spec issues folded in
- 2026-10-31: Started on branch multi-root
- 2026-10-31: Assumption: criterion 16 left unticked - picker.js and the live reload client script are not exercised in a real browser, only their server side; criterion 17 shutdown is the unchanged global handler, untested. Manual step 5: Go mux answers /a/../b/x.md with 307 to /b/x.md, not 301; never 200.
- 2026-10-31: Status: in-review
- 2026-10-31: Review round 1 triage: fixed explicit --port 0 beating config port; added tests for subdirectory .. prefix and watcher-only gone flag. Dismissed: equivalent mutants (port in reserved map, /private/var on macOS), remove-watch with trailing-slash dir (RemoveWatch matches exact key), untested browser-open URL/noOpen branches. Escalated: picker.js and live reload client root= query have no browser test.
- 2026-10-31: Review round 1 answers (Scott Schlesier): accepted the risk that picker.js and the live reload client JS were not run in a browser; keep '<file>:0: no roots defined' for zero roots; agreed the 307-vs-301 clearing was right.
- 2026-10-31: Status: done: Reviewed round 1: all 19 criteria met; criterion 16's client JS (picker.js, live reload) not run in a browser, risk accepted by Scott Schlesier
