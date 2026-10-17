# mdserver

A lightweight, modern Go replacement for the Node-based `markserv` tool that quickly serves Markdown content as HTML.

## Installation

```bash
go install mdserver@latest
```

## Usage

```bash
# Serve current directory (default)
mdserver

# Serve a specific directory
mdserver --dir /path/to/markdown/files

# Serve a specific markdown file
mdserver --file README.md

# Custom host and port
mdserver --host localhost --port 8080

# Auto-select available port (default)
mdserver --port 0

# Enable verbose watcher diagnostics
mdserver --verbose

# Show version information
mdserver --version
```

## Features

- Fast Markdown to HTML rendering with GitHub Flavored Markdown support
- Image serving (`.png`, `.jpg`, `.jpeg`, `.gif`, `.svg`, `.webp`); other non-Markdown files get 404
- Directory index browsing
- Quick file search: press `Alt+O` (`⌥O` on macOS) or click the header's Search button to fuzzy-match Markdown file names under the served directory
- Auto port selection
- Single binary distribution

## Flags

- `--dir` - Directory to serve (default: current working directory)
- `--file` - Specific markdown file to serve (optional)
- `--host` - Host to bind to (default: "localhost")
- `--live-reload` - Enable live reload (default: true)
- `--no-open` - Don't open browser on startup
- `--port` - Port to bind to (default: 0 for auto-selection)
- `--render`, `-r` - Render markdown to HTML and output to stdout
- `--verbose` - Enable verbose watcher and live reload diagnostics
- `--version` - Show version information and exit
