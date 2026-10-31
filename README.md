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

# Serve several roots from one port (see "Multiple roots" below)
mdserver --multi-root [config]
```

## Multiple roots

`mdserver --multi-root [config]` serves several named directories from one port, each at `/<name>/`. Without `--multi-root`, mdserver serves one directory at `/` exactly as before; no config file is ever read.

The roots come from a plain-text config file, read once at startup. `config` is optional; the default is `$XDG_CONFIG_HOME/mdserver/roots` when `XDG_CONFIG_HOME` is set and that file exists, otherwise `~/.config/mdserver/roots` (on macOS too). If the file doesn't exist, mdserver exits with status 1 and lists the paths it looked at.

```text
# Blank lines and lines starting with # are ignored.
port 8099
notes ~/Documents/notes
work  /Users/me/src/project/docs
```

- `port <n>` (optional, once) sets the port. `--port` wins over it; with neither, a free port is chosen.
- Every other line is `<name> <path>`. The name is the first word and uses lowercase letters, digits, `-` and `_`, starting with a letter or digit. `port`, `settings`, `livereload`, `favicon.ico`, `favicon.svg` and `_mdserver` are reserved. The path is the rest of the line, so it may contain spaces; there is no quoting and no inline comments.
- A path must start with `/` or `~/` (only a leading `~/` is expanded), and must be an existing directory. After symlinks are resolved it must not be `/`, your home directory, a parent of it, or one of `/Users`, `/home`, `/etc`, `/var`, `/usr`, `/bin`, `/sbin`, `/opt`, `/private`, `/private/var`, `/private/etc`, `/System` or `/Library`. Subdirectories of those are fine.
- An invalid line, or a file with no roots, stops mdserver with status 1 and a message `<file>:<line>: <reason>`.
- `--multi-root` can't be combined with `--dir`, `--file` or `--render`.

`/` lists the roots. `/<name>/` is that root's directory listing, and root-relative links in its Markdown (`[x](/sub/page.md)`) point inside the root (`/<name>/sub/page.md`). Live reload, the `Alt+O` file search and the settings page work per root. If a root's folder disappears, its pages answer 410 "Folder missing" until mdserver is restarted; the other roots keep serving.

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
- `--multi-root` - Serve several named roots from one port, read from a config file (optional `[config]` argument; see "Multiple roots")
- `--no-open` - Don't open browser on startup
- `--port` - Port to bind to (default: 0 for auto-selection)
- `--render`, `-r` - Render markdown to HTML and output to stdout
- `--verbose` - Enable verbose watcher and live reload diagnostics
- `--version` - Show version information and exit
