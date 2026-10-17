---
title: Add an Alt-O quick-open file picker
type: feature
priority: 2
depends-on: []
parent:
approved: "Scott Schlesier, 2026-10-17: approved after cold read; shortcut Alt-O, sahilm/fuzzy approved. Cold read: pass"
status: in-progress
---

Pressing Alt-O on any mdserver page opens a dropdown where typing fuzzy-matches the names of
all Markdown files under the served root, and choosing a result opens that file.

Context: today the only way to reach a file is to click through directory listings or edit the
URL. Obsidian's quick switcher and VS Code's Ctrl-P are the reference. mdserver runs for one
person on their own machine, so matching runs in the server process and the browser only
renders the top matches. The server is stateless about files: live reload only watches
directories that have been visited (`EnsureWatching`), so it cannot keep a file index fresh,
and the project avoids background polling.

Out of scope:

- Searching file contents, headings or titles. Names (root-relative paths) only.
- Images, directories, and any file mdserver doesn't open as a page.
- A recent-files list, a "create new file" action, or any write to disk.
- Additional or configurable shortcuts.
- `--render` mode, and the settings page.
- Caching or a background index of the file list.

## Acceptance criteria

- [ ] `GET /_mdserver/files?q=<query>` returns `200` with a JSON array of at most 10 objects
      `{"path": "<root-relative path, forward slashes>", "href": "<URL path>", "matches": [<indexes of
      matched characters in path>]}`, best match first, and `Content-Type: application/json`. No
      CORS headers are sent.
- [ ] Matching is fzf-style: a query matches a file when its characters appear in order in the
      file's root-relative path, case-insensitively (`rdme` matches `docs/README.md`). Files
      that don't match are not returned. Ranking rewards consecutive characters and matches at
      word starts and path separators. Pinned by tests: `readme` ranks `README.md` above
      `docs/old-readme-notes.md`, and `ghi` ranks `guides/home-install.md` above
      `guides/zzghizz.md`. Ties break by path, alphabetically.
- [ ] An empty or missing `q` returns the first 10 files alphabetically by path.
- [ ] Only files that mdserver opens as a page (today: `.md` in any case) are returned. Files
      and directories with a dot segment anywhere in their root-relative path are never
      returned. Nothing outside the root is returned. The picker and the page handler use one
      shared predicate for "is a page file", so widening it later changes both.
- [ ] A new file added to the root appears in results on the next request without restarting
      the server or visiting its directory.
- [ ] Every rendered page and directory listing shows a "Search files" button with a hint of the
      shortcut (`⌥O` on macOS, `Alt+O` elsewhere) in the header row next to the version and settings icon. Clicking it
      opens the same overlay. The overlay's empty input has the placeholder "Search files…".
- [ ] On rendered Markdown pages and directory listings, Alt-O opens a centered overlay with a
      focused text input and a result list, and no "ø" is typed into the input and no browser dialog or menu appears.
      Alt-O while it is open closes it.
- [ ] Typing updates the list (requests debounced to at most one per 50 ms, and a stale response
      never overwrites a newer one). The first result is highlighted. Up/Down and Ctrl-N/Ctrl-P
      move the highlight; Enter or a click on a result navigates to its `href`; Esc or a click
      outside closes the overlay without navigating.
- [ ] With no matches the overlay shows "No matching files". If the request fails it shows
      "Search unavailable".
- [ ] The matching characters of each result (from `matches`) are visibly emphasized, and the
      overlay follows the page's existing colors, including dark mode if `style.css` supports it.
- [ ] The picker's script is served from `/_mdserver/picker.js` (embedded in the binary, like
      `style.css`) and is loaded on page and directory templates. It works with live reload
      off.

## Verification

- `go test ./...` passes, with new tests for: the matcher (subsequence, case, the two pinned
  ranking examples, tie-break, `matches` indexes), the page-file predicate, the endpoint (limit of 10, empty query, dotfile and dot-directory
  exclusion, `.MD` included, `.png` and `.txt` excluded, a file added after server start is
  found, no `Access-Control-*` header), and the picker script being served with a JS content
  type.
- `go build . && ./mdserver --dir <a folder with nested docs and a dot-directory>`. In a
  browser: click the header button and press Alt-O on a document page and on a directory page; type a few letters of a
  nested file; confirm ranking, highlight emphasis, keyboard navigation, Enter, Esc, and that
  no "ø" is typed and no browser dialog or menu appears. Repeat in a second browser (Chrome and Firefox, or Safari). Screenshot of the open overlay with
  results for the review, and one of the header button.
- The Alt-O, "ø", dialog, debounce and keyboard criteria have no JS test harness in the repo.
  They are proven by the manual browser steps above, and the PR review records them as manual
  evidence (screenshots), not as automated tests. Go tests cover only that the script and button
  are served.
- Create a new `.md` file while the page is open, press Alt-O, and confirm it can be found.

## Design

- **Endpoint** `/_mdserver/files` sits under the existing reserved `/_mdserver/` prefix, so it
  can't shadow a root file. It's an internal UI endpoint, not a public API. Method `GET` only.
- **No index.** Each request walks the root, filtering with the same rules as the rest of the
  server (`hasDotSegment`, `isValidPath`, case-insensitive `.md`), then scores and sorts. For one
  user and a docs folder this is cheap, results are never stale, and nothing runs in the
  background. The UI debounces keystrokes.
- **Walk limits:** stop after 50,000 directory entries visited so a huge root can't pin the
  server. If the cap is hit, return what was found.
- **Symlinks:** follow symlinks to directories and files inside the root, as the rest of the
  server does (accepted risk in the review profile), with a visited-real-path set to break
  cycles. Results use the path as seen under the root, not the link target.
- **Fuzzy library:** `github.com/sahilm/fuzzy` (MIT, v0.1.3, pure Go). It does fzf-style
  subsequence matching with bonuses for adjacency, separators and first characters, and returns
  the matched character indexes, which go to the client for highlighting. It is not fzf's own
  algorithm. The implementer must check whether its indexes are byte or rune offsets and convert
  to UTF-16 or character positions the client can use. If it fails a pinned ranking example,
  try fzf's `algo` package or tune ordering around it before hand-rolling a matcher.
- **Page-file predicate:** extract the `.md` check used by `handleRequest` into one function
  (e.g. `isPageFile(path)`) and call it from the request handler, the duplicate `.md` check in
  `handleIndex` (`server/handlers.go`) and the picker walk. This
  is a refactor with no behavior change.
- **Discoverability:** a small button in the existing header row (`.breadcrumbs`, beside the
  version and settings icon) with a search icon, the text "Search" and the shortcut hint. It
  opens the overlay and is a real `<button>` so it's keyboard-reachable. Pages without
  breadcrumbs (the root listing has them; nothing else is expected to lack them) show no
  button.
- **Shortcut choice:** Alt-O (Option-O on macOS), chosen to avoid the browser's Open File
  (Cmd-O on macOS, Ctrl-O elsewhere) and other common browser hotkeys. To my knowledge no
  major browser binds Alt-O, but that is from memory and untested, so Verification checks it
  in a second browser. On macOS, Option-O types "ø", so the handler matches on the physical key
  (`event.code === 'KeyO'`), not `event.key`. If a browser's menu mnemonics take Alt-O on
  Windows or Linux, the header button is the fallback.
- **Key handling:** a `keydown` listener on `document` for `altKey && code === 'KeyO'` (no ctrl,
  meta or shift) calls `preventDefault()`. It also works when focus is in an input, and must not
  insert "ø" there.
- **Shortcut label:** the header button shows `⌥O` on macOS and `Alt+O` elsewhere, picked from
  `navigator.platform`/`userAgent`; a wrong guess only changes the label.
- **Where it loads:** `page.html` and `directory.html`. Not `settings.html`.
- **Output safety:** result text is inserted with `textContent` or escaped before any markup, so
  file names can't inject HTML.
- **Live reload:** a reload while the overlay is open simply closes it; no state is kept.
- **Frontend:** plain JS and CSS, no framework or build step. Styles go in `style.css`.

## Steps

1. `server/`: extract the shared page-file predicate; add the root walk that yields candidate
   paths using the existing path rules and the matcher (`fuzzy.go`) built on
   `sahilm/fuzzy`. Add the module to `go.mod`.
2. `server/server.go`: register `/_mdserver/files` and `/_mdserver/picker.js`; embed
   `template/picker.js`.
3. Add `template/picker.js`, picker and header-button styles in `template/style.css`; add the
   button to the header in `page.html` and `directory.html` and load the script there.
4. Tests as listed in Verification. Add a line to `README.md` listing the shortcut.

## Boundaries

Stop and ask if: serving the file list would need any change to which files the server serves,
or if `sahilm/fuzzy` can't meet the pinned ranking examples and the fallbacks in Design don't
either. `sahilm/fuzzy` is the one approved new dependency.

Don't touch: the serving rules in `handleRequest` / `handleStaticFile` beyond extracting the
page-file predicate (and using it in `handleIndex`), the live reload
watcher, or the shutdown endpoints.

## Log

- 2026-10-17: Approved. Scott Schlesier, 2026-10-17: approved after cold read; shortcut Alt-O, sahilm/fuzzy approved. Cold read: pass
- 2026-10-17: Started on branch quick-open-file-picker
