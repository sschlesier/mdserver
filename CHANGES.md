# Changelog

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
