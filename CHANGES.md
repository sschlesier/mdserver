# Changelog

## v3.1.0

- Add a quick file picker: press Alt-O (or use the new header button) to fuzzy-search the Markdown pages and directories under the served root and jump to one
- Add `/_mdserver/files` search endpoint that backs the picker; it lists only pages and directories, never dotfiles, and follows symlinks inside the root
- Document the quick file search in the README

## v3.0.0

- Show the version in the page header beside the settings icon
- Inject the live reload script only when live reload is running, so pages served without it don't load a dead script
- Never serve dotfiles or dot-directories at any depth: any path with a dot segment gets 404
- Shut down when the served directory is renamed, even if another directory replaces it, and wait up to 2s for in-flight requests when the root goes missing
- Fix a panic when the server is stopped twice, and keep a stuck live-reload client from blocking shutdown
- **Breaking:** `/assets/<path>` now serves `<root>/assets/<path>`, so relative links into a folder's own `assets/` directory work. It used to serve `<root>/<path>`. mdserver's own stylesheet moves to `/_mdserver/style.css`, so a folder's `assets/style.css` is no longer shadowed
- mdserver's stylesheet, page templates and favicon are built into the binary, so pages look the same wherever it runs. Homebrew installs, which never shipped `template/`, now get the full styling instead of a plain fallback. A `template/` folder beside the binary or in the working directory is no longer read, so it can't be used to override them
- Every response tells the browser not to cache it (`Cache-Control: no-cache, no-store, must-revalidate`), so an open tab always shows current content. The favicon used to be cached for an hour
- Serve only Markdown (rendered) and images (`.png .jpg .jpeg .gif .svg .webp`) from the served directory; any other file, such as `.html`, `.js`, `.json` or `.txt`, gets 404
- Render `.md` files in any case (`.MD`, `.Md`)
- Serve images with `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; form-action 'none'` and `X-Content-Type-Options: nosniff`, so scripts and forms in an SVG opened directly don't run

## v2.1.0

- Shut down when the served directory is deleted or moved: the next request gets a "folder missing, shutting down" page, and with live reload on the server exits as soon as the file watcher notices

## v2.0.2

- Fix settings icon right-alignment in Firefox
- Add spacing between home icon and path in breadcrumb navigation

## v2.0.1

- Fix live reload for atomic saves (editors that write via temp file + rename)

## v2.0.0

- **Breaking:** Rename `--livereload` flag to `--live-reload` for GNU/POSIX consistency
- Add settings page with server shutdown and file watcher management
- Update README with all flags and show `--` prefix in help output
- Add `/release` slash command for automated release workflow

## v1.3.2

- Auto-open browser on startup; use `--no-open` flag to disable

## v1.3.1

- Fix crash when serving large directory trees (e.g. `$HOME` or `~/src`) by lazy-expanding file watches on demand instead of recursively watching all subdirectories at startup

## v1.3.0

- Add `--render` / `-r` flag for standalone HTML output to stdout
- Fix Homebrew tap scripts: version property without `v` prefix, URLs with `v` prefix

## v1.2.0

- Replace breadcrumb `./` with home icon for larger click target
- Add markdown favicon and improve Safari compatibility

## v1.1.4

- Correct URL handling with `v` prefix for version

## v1.1.3

- Fix Homebrew push token secret reference

## v1.1.2

- Use a PAT for Homebrew tap updates

## v1.1.1

- Automate updating Homebrew tap

## v1.1.0

- Add Mermaid diagram rendering support

## v1.0.0

- Initial release
- Markdown-to-HTML rendering with GitHub Flavored Markdown
- Directory index browsing
- Static asset serving
- Live reload via WebSocket
- Auto port selection
- Cross-platform binaries (macOS, Linux, Windows)
