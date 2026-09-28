---
title: Serve each root directory under a /<name>/ URL prefix
type: feature
priority: 2
depends-on: []
parent:
approved:
---

One mdserver instance can serve several root directories, each under its own
`/<name>/` URL prefix. This holds even with a single root, so URLs don't change when
more roots are added later.

Context: today `Config.RootDir` is a single directory mounted at `/`. Serving a second
directory means starting a second instance on another port. This spec changes the URL
scheme and allows several roots at startup. Adding a root to a running instance is
[[attach-root-to-running-instance]], which depends on this spec.

Out of scope:

- Adding or removing roots while the server runs (that's the follow-up spec).
- Scoping live reload per root. Any change still reloads every open tab, as today.
- CSRF protection for the existing `/settings/*` POST endpoints.
- `--render` mode, which never used the server.

## Acceptance criteria

- [ ] `mdserver --dir ~/notes` serves `~/notes/a.md` at `/notes/a.md` and the listing at `/notes/`.
- [ ] `/notes` (no trailing slash) redirects to `/notes/`.
- [ ] `/` shows a "roots" listing page with one entry per root, linking to `/<name>/`, even when there is only one root.
- [ ] `--dir` can be repeated: `mdserver --dir ~/notes --dir ~/work/docs` serves `/notes/` and `/docs/`.
- [ ] Without `--dir`, the current directory is the only root, named after its basename.
- [ ] A root named after its basename that collides with an existing root or a reserved name gets a `-2`, `-3`, … suffix. Reserved: `assets`, `settings`, `livereload`, `favicon.ico`, `favicon.svg`, `_mdserver`.
- [ ] A root whose basename is empty or `/` (e.g. `--dir /`) is named `root`, with the same collision rule.
- [ ] Passing the same directory twice (after `filepath.Abs` + `filepath.EvalSymlinks`) mounts it once.
- [ ] Startup prints one `Serving <abs path> at <url>/<name>/` line per root, and the browser opens at the first root's `/<name>/`, not `/`.
- [ ] Directory listings, `..` entries and breadcrumbs link under the root's prefix. Breadcrumbs are: home icon → `/`, then `<name>/`, then subdirectories, then the file.
- [ ] The `..` entry at a root's top level links to `/`.
- [ ] Root-relative links and images in rendered Markdown (`[x](/sub/b.md)`, `![](/img/a.png)`) resolve inside the page's own root: the rendered `href`/`src` is `/<name>/sub/b.md`. Relative links, absolute URLs (`https://…`), protocol-relative links (`//…`) and fragments (`#x`) are unchanged.
- [ ] A request whose first path segment matches no root and no reserved route redirects (302) to `/<first-root>/<same path>` if that file or directory exists in the first root. Otherwise it returns 404. This keeps pre-v3 bookmarks working.
- [ ] Path traversal out of a root never returns 200 on the raw request (without following redirects). 301 from the mux's path cleaning, 403 and 404 are all acceptable. A filesystem directory that isn't mounted as a root can't be reached by any URL (404).
- [ ] Live reload watches every root. A save in any root reloads open tabs.
- [ ] The settings page groups watched directories under their root (displayed as `<name>/<rel>`). Each root's top-level watch can't be removed, as today.
- [ ] `server.Config` takes `Roots []Root` (`Name`, `Dir`) instead of `RootDir`. Existing tests are updated to the new URLs and pass.
- [ ] README, `--help` text and CHANGES.md describe the new URL scheme and the repeatable `--dir`. CHANGES.md marks it **Breaking** under a new `v3.0.0` heading.

## Verification

- `go test ./...` and `go build .` pass (same as CI).
- New tests in `server/` cover: prefix routing, the `/` roots page, name collision and reserved names, duplicate dirs, redirect fallback, traversal between roots, root-relative link rewriting, and breadcrumbs for a nested file.
- Manual:
  1. `mkdir -p /tmp/a/sub /tmp/b && printf '# A\n\n[b](/sub/b.md) ![i](/i.png)\n' > /tmp/a/x.md && echo '# B' > /tmp/a/sub/b.md && echo '# other' > /tmp/b/x.md`
  2. `go run . --dir /tmp/a --dir /tmp/b --port 8099`. The browser opens `http://localhost:8099/a/`.
  3. `http://localhost:8099/` lists `a/` and `b/`.
  4. `/a/x.md`: the `b` link goes to `/a/sub/b.md` and the image src is `/a/i.png`.
  5. `/x.md` redirects to `/a/x.md`.
  6. Edit `/tmp/b/x.md` while `/b/x.md` is open: the tab reloads.
  7. `curl -s -o /dev/null -w '%{http_code}' --path-as-is http://localhost:8099/a/../../etc/hosts` prints `301` (the mux cleans the path) or `404`/`403`. It never prints `200`.

## Design

Flags: **public API**. The URL scheme and the `server.Config` shape change, and
`--dir` becomes repeatable. This is a major version bump (v3.0.0), in line with how
v2.0.0 handled the flag rename.

Decisions:

- **Always prefix, even with one root.** URLs don't depend on how many roots are
  mounted, so an open tab keeps working when a second root is attached later.
- **Name = basename of the absolute dir**, deduplicated with `-N`. No `name=path`
  override in this spec. It can be added later if basename collisions turn out to be
  annoying.
- **Root order** is the order given. The first root is the "primary" one, used by the
  legacy redirect and the browser-open URL.
- **Legacy redirect** uses 302 rather than 301, so browsers don't cache it
  permanently. It only fires when the target exists in the primary root.
- **Root-relative link rewriting** happens at render time, in the server path only.
  `renderer.RenderMarkdown` gets an option (e.g. `RenderMarkdownWithBase(content,
  "/<name>")`) implemented as a goldmark AST transformer on `ast.Link` and `ast.Image`
  destinations that start with a single `/`. `RenderStandalone` and `--render` don't
  change. Raw HTML `<a href="/…">` inside Markdown isn't rewritten; the legacy
  redirect covers it for the primary root only.
- **Reserved routes** (`/assets/`, `/settings…`, `/livereload`, `/favicon.*`,
  `/_mdserver/`) keep their absolute paths. `/assets/style.css` still serves the CSS.
  `/assets/<file>` currently serves root files as a side route. It now resolves
  against the primary root, which keeps it backward compatible.
- **Live reload** stays one `LiveReload` with one fsnotify watcher and one client
  set. `NewLiveReload` takes a list of root dirs, and `Start` watches each one to
  depth 1. Add an `AddRoot(dir)` method now, because the follow-up spec needs it.
- **`--file`** is currently only logged and doesn't affect serving. Leave it as is.
- **Type:** `type Root struct { Name, Dir string }`. Resolve roots through one
  `(*Server).resolve(urlPath) (root *Root, rel string, ok bool)` helper so
  `isValidPath`, `relPath`, breadcrumbs and listings all take the root explicitly
  instead of reading `s.config.RootDir`.
- **Cross-root links:** a root-relative link always resolves inside the page's own
  root. `/b/x.md` written in root `a` becomes `/a/b/x.md`. To link to another root,
  use a full URL.
- **Naming inputs:** `Root.Dir` stores the `filepath.Abs` path, and the name comes
  from its basename. `EvalSymlinks` is only the dedup key. A non-empty `Root.Name`
  in `Config` is used as the base name and still deduplicated. The CLI never sets one.
  Names that need escaping are `url.PathEscape`d in every URL.
- **Nested roots** (`--dir ~/n --dir ~/n/sub`) are allowed and mounted separately.
- **`/<name>` → `/<name>/`** uses 301, like today's directory redirects. Reserved
  first segments never take the legacy redirect.
- **Roots slice** is guarded by a `sync.RWMutex` on `Server`, ready for runtime adds
  in the follow-up.

## Steps

1. `server`: add `Root`, `Config.Roots`, the naming/dedup helper (`AddRoot` on
   `Server` returning `(name string, existed bool)`) and the mutex. Remove `RootDir`.
2. `server`: route `/<name>/…` through `resolve`, add the `/` roots page (reuse
   `directory.html` with root entries), add the legacy 302 fallback, and make
   `isValidPath`/`relPath` per root.
3. `server`: make breadcrumbs and listing URLs take a prefix. Make the `..` entry at a
   root's top level link to `/`.
4. `renderer`: add the base-path link rewriting option, with tests.
5. `livereload`: support several roots and add `AddRoot`. Group the settings page by
   root.
6. `main.go`: make `--dir` repeatable (a custom `flag.Value`), print one line per root,
   and open the browser at the first root.
7. Update existing tests. Add the new tests listed in Verification.
8. README, `--help` and CHANGES.md (`v3.0.0`, **Breaking**).

## Boundaries

Stop and ask if: a route, template or test outside those named here depends on
`/`-rooted file URLs in a way the legacy redirect doesn't cover.
