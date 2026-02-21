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

## Releasing

Follow [semver](https://semver.org/) when choosing the increment:

- **patch** — bug fixes, minor UI tweaks, internal refactors with no behavior change
- **minor** — new features, new UI capabilities, non-breaking additions
- **major** — breaking changes to CLI flags, config, or workflows that require user action

Steps:

1. Determine the latest tag and list commits since that tag
2. **Ask the user** to choose patch / minor / major before proceeding (use AskUserQuestion with the three options; include the computed next version in each option's description)
3. Update `CHANGES.md` — add a section for the new version with user-facing changes at the top of the file
4. Commit: `git commit -am "Prepare release vX.Y.Z"`
5. Tag: `git tag vX.Y.Z`
6. Push: `git push origin main && git push origin vX.Y.Z`

The `v*` tag push triggers the GitHub Actions release workflow which:
- Runs tests
- Cross-compiles binaries (macOS amd64/arm64, Linux amd64/arm64, Windows amd64)
- Creates a GitHub release with binaries and checksums
- Updates the Homebrew tap (`sschlesier/homebrew-mdserver`)

```bash
# Example: patch release
git commit -am "Prepare release v1.3.1"
git tag v1.3.1
git push origin main && git push origin v1.3.1
```

- Always update `CHANGES.md` before tagging
- Never manually edit the version in `main.go` — it's injected via ldflags at build time
- The tag name (e.g., `v1.3.1`) is the single source of truth for the version
