# CLAUDE.md — Quartermaster

## This is a Public Open-Source Repository

This repository is publicly visible and Apache 2.0 licensed. Every file committed here is
open to the world. Code from the private Minions monorepo must **never** be added unless it
has been explicitly reviewed and placed on the whitelist below.

## Commit and Push Policy

**CRITICAL: You MUST get explicit human approval before making any commit or push to this
repository — including on non-main branches.**

Before committing or pushing anything, stop and ask:
> "I'm about to commit [X]. This is a public repo. Please confirm."

The human must reply with an explicit "yes" or "go ahead" before you proceed. Do not
interpret silence, context, or prior approval of related work as permission.

## Content Whitelist

Only the items listed here may be committed. Anything else — a new file, a new dependency,
a new build step, a new pattern — requires the human to explicitly say "add it to the
whitelist" before it may be committed or pushed, even on a feature branch.

### Installer application source

| File / path | What it is |
|-------------|-----------|
| `main.go` | Wails entry point |
| `app.go` | App struct; installation step orchestration |
| `config.go` | Install-location config persistence (`%APPDATA%\Minions\`) |
| `downloader.go` | HTTP download, SHA-256 verification, zip extraction |
| `git_auth.go` | Git credential helper setup |
| `git_installer.go` | Git for Windows / Homebrew install |
| `claude_installer.go` | Claude Code native installer |
| `launcher.go` | Volta/Node.js install; Dominion process launch; login-item registration |
| `launcher_unix.go` | Unix-specific background process spawn |
| `launcher_windows.go` | Windows-specific `CreateProcess` (no-console window) |
| `oauth.go` | OAuth device-flow (GitHub, GitLab) and PKCE (Bitbucket) — client IDs injected via ldflags, no secrets in source |
| `go.mod` | Go module declaration (`github.com/Minions/quartermaster`) |
| `go.sum` | Go dependency checksums |
| `wails.json` | Wails v2 app configuration |

### Frontend UI

| File / path | What it is |
|-------------|-----------|
| `frontend/index.html` | Installer UI markup |
| `frontend/main.js` | Installer UI logic (vanilla JS, no framework) |
| `frontend/style.css` | Installer UI styles |

### Build and CI

| File / path | What it is |
|-------------|-----------|
| `.github/workflows/release.yml` | Multi-platform build (Windows, macOS arm64/x86), SHA-256 checksums, GitHub Release creation. SignPath signing stub included (commented out until enrolled). |

### Repo boilerplate

| File / path | What it is |
|-------------|-----------|
| `LICENSE` | Apache License 2.0 |
| `README.md` | Project description |
| `.gitignore` | Excludes `build/`, compiled binaries, OS files |
| `CLAUDE.md` | This file |

## What Is NOT Allowed (examples)

- Any code from `work/local/` (the private monorepo) not on the list above
- The Dominion runtime, cabinet, throne-room, conductor-cli, or any other private app
- Costumes, paid features, or any monetisation logic
- Internal tooling, scripts, or configuration from the private monorepo
- Telemetry, analytics, or remote logging
- Any hardcoded secrets, API keys, or credentials
- New dependencies (Go modules, npm packages) without review
- New build steps, scripts, or CI jobs beyond what's listed above

## Adding to the Whitelist

If you believe something new should be added:
1. Stop — do not commit or push.
2. Describe what it is, why it belongs in the public repo, and confirm it contains no
   proprietary logic or secrets.
3. Wait for the human to explicitly say "add it to the whitelist" and update this file.
4. Only then may you commit.
