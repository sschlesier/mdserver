---
title: Push the Homebrew tap with a deploy key instead of a PAT
type: chore
priority: 1
depends-on: []
approved: "Scott Schlesier, 2026-09-30: approved with hand-update to v2.1.0 and Claude-run key setup. Cold read: pass"
status: in-review
---

A tagged mdserver release updates the Homebrew tap again, authenticated by a deploy key on
sschlesier/homebrew-mdserver that doesn't expire, instead of the expired
`HOMEBREW_PUSH_TOKEN` PAT.

Context: release run 36478916209 (v2.1.0, 2026-09-28) built and published the GitHub
release with all 6 assets, then failed in "Update Homebrew Tap" at "Checkout
homebrew-mdserver" with `Bad credentials`. The `HOMEBREW_PUSH_TOKEN` secret was last set
2026-04-17, so the PAT has most likely expired. The tap's `Formula/mdserver.rb` is still at
v2.0.2. dgrid made the same move in sschlesier/dgrid 85e7907 ("ci: use SSH deploy key
instead of PAT for homebrew tap push"): `actions/checkout` with
`ssh-key: ${{ secrets.HOMEBREW_DEPLOY_KEY }}`, a plain `git push origin main`, and a
write-enabled deploy key `homebrew-dgrid-deploy` on sschlesier/homebrew-dgrid.
homebrew-mdserver has no deploy keys today.

Out of scope:

- Bumping action versions (`actions/checkout@v4` etc.) for the Node 20 deprecation
  warnings, or `softprops/action-gh-release@v1`.
- Any other change to the test, build or release jobs, or to `scripts/`.
- Rewriting the formula update logic.
- Re-running the failed v2.1.0 run: a re-run uses the workflow as of the v2.1.0 tag, which
  still uses the PAT.
- Cutting a new release. The workflow change is proven end to end by the next release.
- CHANGES.md: it has no unreleased section; the next release's section can mention this.

## Acceptance criteria

- [ ] `.github/workflows/release.yml` no longer references `HOMEBREW_PUSH_TOKEN`; the
      homebrew-mdserver checkout uses `ssh-key: ${{ secrets.HOMEBREW_DEPLOY_KEY }}` and the
      push step runs `git push origin main` with no `remote set-url`.
- [ ] sschlesier/homebrew-mdserver has one deploy key titled `homebrew-mdserver-deploy` with
      write access (`read_only=false`).
- [ ] The tap's `Formula/mdserver.rb` on main reports `version "2.1.0"` with the v2.1.0
      release's checksums, in a commit `Update mdserver to v2.1.0` pushed over SSH with that
      deploy key.
- [ ] sschlesier/mdserver has an Actions secret `HOMEBREW_DEPLOY_KEY` holding that key's
      private half; the key files generated in the scratchpad are deleted (`ls` on their
      paths fails).
- [ ] The release workflow still triggers only on `v*.*.*` tag pushes and still changes only
      `Formula/mdserver.rb` in the tap.
- [ ] CLAUDE.md's review profile names `HOMEBREW_DEPLOY_KEY` (a write deploy key scoped to
      sschlesier/homebrew-mdserver) instead of `HOMEBREW_PUSH_TOKEN`.

After merge:

- [ ] The `HOMEBREW_PUSH_TOKEN` secret is deleted from sschlesier/mdserver.

## Verification

- `go test ./...` and `go build .` pass (no Go changes expected; confirms nothing else moved).
- `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/release.yml`
  → no findings on the changed lines.
- `grep -rn HOMEBREW_PUSH_TOKEN --exclude-dir=.git --exclude-dir=tickets .` → no matches.
- `git diff main -- .github/workflows/release.yml` → only the update-homebrew job changes;
  `on:` is untouched.
- `gh api repos/sschlesier/homebrew-mdserver/keys -q '.[] | "\(.title) \(.read_only)"'`
  → `homebrew-mdserver-deploy false`.
- `gh api repos/sschlesier/homebrew-mdserver/commits -q '.[0].commit.message'` →
  `Update mdserver to v2.1.0`; the formula's sha256 values match the v2.1.0 release's
  `checksums.txt`.
- `gh secret list -R sschlesier/mdserver` → `HOMEBREW_DEPLOY_KEY` present; after merge,
  `HOMEBREW_PUSH_TOKEN` absent.
- `brew update && brew upgrade sschlesier/mdserver/mdserver && mdserver --version`
  → reports v2.1.0 (manual, on the user's machine).

## Design

Flags: **config change** (new deploy key on the tap repo, new Actions secret, removal of the
old secret).

- Mirror dgrid exactly: secret name `HOMEBREW_DEPLOY_KEY`, key title
  `homebrew-mdserver-deploy`, `ssh-key:` on the `actions/checkout` step for the tap, plain
  `git push origin main` (checkout configures SSH for the push).
- Key: `ssh-keygen -t ed25519 -N "" -C homebrew-mdserver-deploy`, generated in the session
  scratchpad by Claude. Public half added with
  `gh repo deploy-key add <pub> -R sschlesier/homebrew-mdserver -t homebrew-mdserver-deploy -w`;
  private half set with `gh secret set HOMEBREW_DEPLOY_KEY -R sschlesier/mdserver < <priv>`.
  Both files deleted afterwards.
- Recover v2.1.0's missing tap update by hand: clone the tap over SSH into the scratchpad
  with `GIT_SSH_COMMAND="ssh -i <priv> -o IdentitiesOnly=yes"`, download the v2.1.0
  release's `checksums.txt`, run
  `scripts/update-homebrew-formula.py v2.1.0 checksums.txt <tap>/Formula/mdserver.rb`,
  commit `Update mdserver to v2.1.0` as github-actions[bot] (matching the workflow's
  commits), push to main. Pushing with the deploy key proves it can write before it goes
  into the secret. No new release; no `workflow_dispatch` trigger is added, keeping the
  "only on a v*.*.* tag push" invariant.
- Leave the `update-homebrew` job's `permissions: contents: write` as is; the deploy key
  does the tap write either way.
- Delete `HOMEBREW_PUSH_TOKEN` after merge. It's expired, so keeping it as a fallback buys
  nothing. Revoking the PAT itself on github.com is the user's job (gh can't revoke a
  classic PAT).

## Steps

1. Generate the key; add the deploy key to the tap.
2. Hand-update the tap to v2.1.0 over SSH with the key.
3. Set the `HOMEBREW_DEPLOY_KEY` secret; delete the local key files and tap clone.
4. Edit `.github/workflows/release.yml` update-homebrew job: `token:` → `ssh-key:`, drop
   `remote set-url` and the `env:` block on "Push changes".
5. Update CLAUDE.md review profile `Data:` line.
6. Run verification; open the PR via `pr-review`.
7. After merge: delete `HOMEBREW_PUSH_TOKEN`; remind the user to revoke the PAT.

## Boundaries

Stop and ask if: a step would change the tap repo in any way other than adding the deploy
key and the v2.1.0 formula commit (branch protection, settings, other files), or the push
to the tap is rejected.

Don't touch: the test, build and release jobs; the `on:` trigger.

## Log

- 2026-09-30: Drafted from failed run 36478916209 and dgrid 85e7907.
- 2026-09-30: Cold read: pass, no blocking. Fixed grep expectation, scoped the key-on-disk
  check to the scratchpad files, split post-merge criteria.
- 2026-09-30: User chose hand-updating the tap to v2.1.0 over releasing v2.1.1, and Claude
  doing the key setup via gh. Push the hand-update with the new deploy key to prove it writes.
- 2026-09-30: Approved: Scott Schlesier, 2026-09-30: approved with hand-update to v2.1.0 and Claude-run key setup. Cold read: pass
- 2026-09-30: Started on branch push-homebrew-tap-with-deploy-key
- 2026-09-30: Deploy key `homebrew-mdserver-deploy` added to the tap (read_only=false).
- 2026-09-30: The worktree-isolated session refuses git aimed at another repo, so the user
  ran the v2.1.0 tap update themselves, from their existing checkout
  `~/src/homebrew-mdserver` instead of a scratchpad clone (pull --ff-only first; push with
  only the deploy key via GIT_SSH_COMMAND). Tap commit c219e81 `Update mdserver to v2.1.0`;
  all four sha256 values match the v2.1.0 `checksums.txt`.
- 2026-09-30: `HOMEBREW_DEPLOY_KEY` secret set; scratchpad key files deleted (`ls` empty).
- 2026-09-30: Assumption: the HOMEBREW_PUSH_TOKEN grep excludes `tickets/`, since this spec
  names it. PyYAML isn't installed; the YAML check used `ruby -ryaml` instead.
- 2026-09-30: Review started
