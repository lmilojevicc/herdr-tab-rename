<div align="center">

# herdr-tab-rename

<p align="center">
    <em>Check out my other Herdr-related projects.</em> <br>
    <a href="https://github.com/lmilojevicc/seshagy"><em>Seshagy - Agent aware TUI session manager</em></a>
    &nbsp; | &nbsp;
    <a href="https://github.com/lmilojevicc/herdr-splits.nvim"><em>herdr-splits.nvim - Seamless nvim and herdr navigation</em></a>
</p>
</div>

A [Herdr](https://github.com/lmilojevicc/herdr) plugin that automatically renames each tab to the basename of its focused pane's current working directory. Tabs you rename manually are left alone for the rest of the session.

## Features

- Names each tab after its focused pane's working directory (`~/Projects/foo` → `foo`).
- Honors manual renames: once you rename a tab, the plugin stops managing it.
- Autostarts with Herdr.
- Zero configuration — cwd-only, always on.

## Requirements

- Herdr >= 0.7.0
- curl or wget (one is preinstalled on macOS and Linux)
- Linux or macOS

Building from source (local development) additionally requires Go.

## Installation

From GitHub (downloads a prebuilt binary — no Go required):

```sh
herdr plugin install lmilojevicc/herdr-tab-rename
# or pin a specific release:
herdr plugin install lmilojevicc/herdr-tab-rename --ref v0.1.0
```

The install step runs `scripts/install.sh`, which downloads the right
prebuilt binary for your OS/arch from GitHub Releases and verifies its
SHA256 checksum.

Local development:

```sh
go build -o herdr-tab-rename .
herdr plugin link .
```

> `herdr plugin link` registers the plugin from the current directory but does **not** run the build step — run `go build` first so the `herdr-tab-rename` binary exists.

## Releases

Binaries are built by GitHub Actions (`.github/workflows/release.yml`) on
every `git tag v*` push and attached to the GitHub Release. Four targets are
produced: linux/macos × amd64/arm64, plus a `SHA256SUMS` file.

## How it works

The daemon polls every 500ms over the Herdr unix socket, reading `tab.list` and `pane.list` and calling `tab.rename` as needed.

- The desired tab label is the basename of the focused pane's `foreground_cwd` (falling back to `cwd`).
- A tab is **locked** the first time the daemon notices its label changed mid-session to something the daemon itself did not set. After that the tab is ignored until the daemon restarts.
- On startup the daemon **seeds** every existing tab's current label as the baseline, so a restart never mass-locks tabs just for existing. Locking only happens for a change observed during the running session.
- Lock state is kept **in-memory only**; a daemon restart resets it.

## Manual control

```sh
./herdr-tab-rename start   # launch the daemon (idempotent; safe to call repeatedly)
./herdr-tab-rename stop    # stop the daemon
```

Logs are written to `~/.local/state/herdr/plugins/herdr-tab-rename/daemon.log`.

## License

MIT
