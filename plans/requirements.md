## Goal

Build a lightweight, modern Go replacement for the Node-based `markserv` tool that quickly serves Markdown content as HTML with live reload.

## Functional Requirements

1. **Markdown rendering**
   - Serve a specified Markdown file or directory index as styled HTML.
   - Support GitHub-flavored Markdown (tables, fenced code, task lists).
   - Provide syntax highlighting for fenced code blocks.
2. **Static asset serving**
   - Serve co-located assets (images, CSS, JS) referenced by the Markdown.
   - Support directory browsing so users can select any Markdown file.
3. **Fast local web server**
   - CLI starts an HTTP server on localhost in under 200 ms on typical hardware.
   - Automatically chooses an available port, exposing the URL to the user.
4. **Live reload**
   - Watch Markdown and asset files for changes.
   - Trigger in-browser refresh via WebSocket or SSE within 250 ms of change.
5. **Routing parity with markserv**
   - Handle relative links the same as markserv to ease migration.
   - Provide optional fallback file (e.g., README.md) when no path given.
6. **CLI ergonomics**
   - Single binary install via `go install`.
   - Defaults to serving the current working directory.
   - Flags for host, port, entry file, theme, and disabling live reload.
7. **Template customization**
   - Allow overriding HTML template and CSS theme via a config file or flags.

## Non-Functional Requirements

1. **Portability**
   - Build and run on macOS, Linux, and Windows without CGO.
2. **Performance**
   - Cache rendered HTML and invalidate when files change.
   - Keep CPU usage near idle when watching files.
3. **Security**
   - Bind to localhost by default; require explicit opt-in to bind publicly.
   - Serve only files within the root directory.
4. **Observability**
   - Structured logging with concise startup summary and reload notifications.
   - Optional verbose mode for debugging file watch events.
5. **Testing**
   - Include unit tests for markdown rendering logic and live reload handler.
   - Provide an integration test that spins up the server and fetches HTML.

## Stretch Goals

1. Built-in dark/light themes with auto-switching based on system preference.
2. Optional directory watch ignore patterns (gitignore-compatible).
3. Ability to export rendered Markdown to static HTML files.
4. Embed assets into the binary for zero-dependency distribution.
