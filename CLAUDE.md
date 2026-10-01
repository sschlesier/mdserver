# mdserver

A lightweight Go Markdown server with live reload.

## Project Structure

- `main.go` - Entry point, CLI flag parsing
- `server/` - HTTP server, handlers, live reload
- `renderer/` - Markdown-to-HTML rendering (goldmark)
- `scripts/` - Homebrew formula update scripts
- `.github/workflows/` - CI and release automation

## Development

```bash
go test ./...       # Run tests
go build .          # Build binary
go run . --dir .    # Run locally
```
## Review profile

Purpose:   Serve a local directory of Markdown as HTML with live reload, for one person
           reading or editing docs in their own browser (a markserv replacement). Also
           renders a single file to stdout (--render). Released via GitHub Releases and a
           Homebrew tap.
Deploy:    The user's own machine. Binds to localhost on an auto-selected port by default;
           --host can expose it to the network.
Load:      One user, a few browser tabs.
Data:      Reads files under the served root; writes nothing to disk. Nothing sensitive of
           its own, but it serves what's in the root. The release workflow holds
           HOMEBREW_DEPLOY_KEY, a write deploy key scoped to sschlesier/homebrew-mdserver.
Staleness: A saved file should show in open tabs within about a second (live reload).

Invariants:
- No URL serves, renders or lists a file or directory outside the served root, except
  through a symlink inside the root.
- Dotfiles and dot-directories under the root are never served.
- Only Markdown (.md, rendered) and image files (.png .jpg .jpeg .gif .svg .webp) under the
  root are served; any other extension gets 404.
- Raw HTML in Markdown isn't passed through to the page.
- The server never writes, moves or deletes files under the root.
- With no flags, it listens only on localhost.
- The release workflow changes only Formula/mdserver.rb in the tap, and only on a v*.*.*
  tag push.

Accepted risks:
- POST /settings/shutdown and /settings/remove-watch have no CSRF check, so any page open in
  the user's browser can stop the server or drop a watch. Impact: annoyance only.
  Accepted by Scott Schlesier, 2026-09-30.
  Valid while: it binds to localhost by default, and those endpoints only stop or unwatch.
- The live-reload WebSocket accepts any Origin, so another site can learn when files change.
  Accepted by Scott Schlesier, 2026-09-30.
  Valid while: messages carry reload signals, not file contents.
- Symlinks inside the root are followed, even when they point outside it. Impact: the server
  shows whatever the user linked into the directory they chose to serve.
  Accepted by Scott Schlesier, 2026-09-30.
  Valid while: it binds to localhost by default.
