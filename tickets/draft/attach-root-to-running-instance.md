---
title: Attach a new root to an already-running mdserver instead of starting a new one
type: feature
priority: 2
depends-on: [serve-roots-under-name-prefix]
parent:
approved:
---

Running `mdserver --dir fee` while another mdserver is already running adds `fee` as a
new root on that instance (`localhost:8080/fee/`) instead of starting a second server
on another port.

Context: the usual flow is `mdserver --dir foo` (on :8080), then later `mdserver --dir
fee`, which today starts a second server on :8081. After
[[serve-roots-under-name-prefix]], one instance can hold several roots. This spec adds
the discovery and the authenticated "add root" call.

Out of scope:

- Removing a root at runtime (from the CLI or the settings page).
- Attaching across hosts, or to an instance started by another OS user.
- Forwarding per-invocation flags (`--live-reload`, `--verbose`, `--file`) to the running instance.
- CSRF protection for the existing `/settings/*` endpoints.

## Acceptance criteria

- [ ] Each running server writes an instance file `<UserConfigDir>/mdserver/instances/<host>_<port>.json` with mode `0600`, containing `{"pid", "host", "port", "token", "version"}`. `token` is 32 random bytes, hex-encoded. The file is removed on clean shutdown (Ctrl+C, SIGTERM, settings shutdown).
- [ ] `POST /_mdserver/roots` with header `Authorization: Bearer <token>` and JSON body `{"dir": "<abs path>"}` adds the root, starts watching it, and returns `200 {"name": "...", "url": "http://host:port/<name>/"}`. The naming rules are the same as at startup. If the dir is already mounted, it returns that root's existing name and URL.
- [ ] That endpoint returns 401 for a missing or wrong token, 400 for a non-directory or relative path, and 405 for methods other than POST. It never sends CORS headers.
- [ ] `GET /_mdserver/info` returns `{"app":"mdserver","version":...}` with no auth. The client uses it to confirm that the port really is an mdserver.
- [ ] With no `--port`, `mdserver --dir fee` looks for instance files matching `--host`. For each one, lowest port first, it checks that the PID is alive and that `/_mdserver/info` answers. It attaches to the first live one. Stale files (dead PID or no answer) are deleted.
- [ ] With `--port N`: if a live instance file exists for `host_N`, it attaches there. If the port is free, it starts a new server on N. If the port is taken by something that isn't an attachable mdserver, it exits non-zero with a clear error (as today).
- [ ] With `--new`, it skips discovery and always starts a new server, using today's port selection.
- [ ] After a successful attach, the CLI prints `Added <abs dir> to <url>`, opens the browser at the new root's URL (unless `--no-open`), and exits 0. No second server keeps running.
- [ ] If the attaching invocation passes `--live-reload`, `--verbose` or `--file`, the CLI prints one warning that these are ignored for an attached root.
- [ ] Several `--dir` flags in one attaching invocation attach each one, and the browser opens at the first.
- [ ] If the attach request fails (401, network error, version mismatch with no `/_mdserver/roots` route), the CLI prints the error and exits non-zero. It does not silently start a new server. The error message suggests `--new`.
- [ ] Open tabs on the `/` roots page see the new root after a reload. A live-reload broadcast on attach is optional, not required.
- [ ] README and CHANGES.md document attach behavior, `--new`, and the instance file location.

## Verification

- `go test ./...` and `go build .` pass. `GOOS=windows go build .` and `GOOS=linux go build .` also pass.
- New tests: the instance file is written with 0600 and removed on `Stop`. `/_mdserver/roots` covers auth (no token, wrong token, correct token), duplicate dir, bad dir, and GET → 405. Discovery skips a stale file with a dead PID and deletes it. `/settings/shutdown` removes the instance file. `--port N` held by a non-mdserver listener exits non-zero. The attach client talks to an `httptest` server (the client logic is factored out of `main` so it can be tested).
- Manual:
  1. `go run . --dir /tmp/a`. Note the port (e.g. 8080) and check that `ls -l ~/Library/Application\ Support/mdserver/instances/` (Linux: `~/.config/mdserver/instances/`) shows `localhost_8080.json` with `-rw-------`.
  2. In a second terminal, `go run . --dir /tmp/b`. It prints `Added /tmp/b to http://localhost:8080/b/`, exits 0, and the browser opens there.
  3. `http://localhost:8080/` lists `a/` and `b/`. Editing `/tmp/b/x.md` reloads its open tab.
  4. `curl -X POST -d '{"dir":"/"}' localhost:8080/_mdserver/roots` returns 401.
  5. `go run . --dir /tmp/b --new` starts a separate server on 8081.
  6. Ctrl+C the 8080 server. The instance file is gone. `kill -9` a server, then run `go run . --dir /tmp/a`: the stale file is deleted and a new server starts.

## Design

Flags: **public API** (new CLI behavior, new `--new` flag, new HTTP endpoints) and
**config change** (new on-disk state under the user config dir).

Decisions:

- **Default is attach.** A plain `mdserver --dir X` attaches if an instance is
  running, and `--new` opts out. This matches the requested workflow. It changes
  today's behavior (a second invocation used to start a second server), which is why
  it ships in v3.0.0 alongside the prefix change.
- **Discovery via instance files, not port probing.** Only files written by the same
  OS user are visible, and they carry the token. Probing `8080..` alone can't tell
  "busy with something else" apart from "mdserver", and can't authenticate.
- **Auth via a bearer token in a 0600 file.** Without it, any web page the user
  visits could POST to `localhost:8080` and mount `~` or `/`, and a DNS-rebinding page
  could then read it. A custom `Authorization` header can't be sent cross-origin
  without a CORS preflight, and the server never answers preflights. The token also
  keeps other local users out.
- **Location:** `os.UserConfigDir()` + `/mdserver/instances/`. Go has no portable
  state dir, and this is per user on all three OSes.
- **PID liveness:** `os.FindProcess` + `Signal(syscall.Signal(0))` on Unix. On
  Windows, treat `FindProcess` success as alive and rely on the `/_mdserver/info`
  probe.
- **Attached roots live as long as the instance.** Stopping the first
  invocation stops every root. The README states this.
- **Version skew:** the client sends its version in `X-Mdserver-Version`. The server
  accepts any version. If `/_mdserver/info` is missing (a pre-v3 server), the client
  errors and suggests `--new`.
- `Server.AddRoot` and `LiveReload.AddRoot` from the dependency are the only
  server-side mutation paths. `Server.AddRoot` returns `(name string, existed bool)`.
- **Write ordering:** bind the listener (`net.Listen` + `http.Serve`) before writing
  the instance file, so the file never names a port the server failed to get.
- **Shutdown cleanup:** `/settings/shutdown` currently calls `os.Exit(0)` directly.
  It must run the same cleanup as `Stop()` (removing the instance file) before exiting.
- **Discovery details:** match `--host` exactly (`localhost` and `127.0.0.1` are
  different). A file whose port answers but isn't mdserver counts as stale. The HTTP
  client times out after 2s. With several `--dir` values, stop at the first failed
  attach; roots already attached stay attached.
- **Ignored-flags warning** fires only for flags set explicitly (`flag.Visit`),
  because `--live-reload` defaults to true.
- **Printed path** in `Added <dir>` is the `filepath.Abs` path, not the
  symlink-resolved one.
- **Windows:** PID checks are split with build tags. The 0600 mode is a no-op there,
  and `%AppData%` being per-user is the protection. The mode test is skipped on
  Windows.

## Steps

1. `server`: add `GET /_mdserver/info` and `POST /_mdserver/roots` with token
   checking. The token comes from `Config.Token`.
2. New package `instance`: write/remove the instance file, list and prune instance
   files, check PID liveness, and an `Attach(ctx, inst, dir)` client.
3. `main.go`: add the `--new` flag. Discovery → attach → exit, or fall through to
   starting a server that writes its instance file. Remove the file on every shutdown
   path, including `/settings/shutdown`.
4. Tests as listed in Verification.
5. README and CHANGES.md.

## Boundaries

Stop and ask if: the instance-file approach doesn't work on one of macOS, Linux or
Windows. Also stop if the token check would need to be relaxed for any reason.
