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