---
description: Prepare and publish a new release
allowed-tools:
  - Bash(git tag:*)
  - Bash(git log:*)
  - Bash(git push:*)
  - Bash(git commit:*)
  - Read
  - Edit
  - Write
  - AskUserQuestion
---

# Release

Follow the release process for mdserver. Execute each step in order.

## Step 1: Determine the latest tag and recent changes

- Run `git tag --sort=-v:refname | head -1` to find the latest tag
- Run `git log <latest_tag>..HEAD --oneline` to list commits since that tag
- If there are no commits since the last tag, stop and tell the user there is nothing to release

## Step 2: Ask the user to choose the version bump

Using AskUserQuestion, present three options: patch, minor, major. Compute the next version for each option and include it in the description. Refer to semver guidelines:

- **patch** — bug fixes, minor UI tweaks, internal refactors with no behavior change
- **minor** — new features, new UI capabilities, non-breaking additions
- **major** — breaking changes to CLI flags, config, or workflows that require user action

## Step 3: Update CHANGES.md

- Read `CHANGES.md`
- Add a new section at the top (after the `# Changelog` heading) for the new version
- Summarize user-facing changes from the commits since the last tag
- Write the updated file

## Step 4: Commit, tag, and push

```bash
git commit -am "Prepare release vX.Y.Z"
git tag vX.Y.Z
git push origin main && git push origin vX.Y.Z
```

Replace `vX.Y.Z` with the chosen version.

## Important rules

- Never manually edit the version in `main.go` — it's injected via ldflags at build time
- The tag name (e.g., `v1.3.1`) is the single source of truth for the version
- Always update `CHANGES.md` before tagging
