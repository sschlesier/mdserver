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

# Show version information
mdserver --version
```

## Features

- Fast Markdown to HTML rendering with GitHub Flavored Markdown support
- Static asset serving (images, CSS, JS)
- Directory index browsing
- Auto port selection
- Single binary distribution

## Flags

- `--host` - Host to bind to (default: "localhost")
- `--port` - Port to bind to (default: 0 for auto-selection)
- `--file` - Specific markdown file to serve (optional)
- `--dir` - Directory to serve (default: current working directory)
- `--livereload` - Enable live reload (default: true)
- `--version` - Show version information and exit

