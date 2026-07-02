# herdr-tab-rename

A [Herdr](https://github.com/lmilojevicc/herdr) plugin that automatically renames each tab to the basename of its focused pane's current working directory. Tabs you rename manually are left alone for the rest of the session.

## Features

- Names each tab after its focused pane's working directory (`~/Projects/foo` → `foo`).
- Honors manual renames: once you rename a tab, the plugin stops managing it.
- Autostarts with Herdr.
- Zero configuration — cwd-only, always on.

## Requirements

- Herdr >= 0.7.0
- Go (to build from source)
- Linux or macOS

## Installation

From the Herdr plugin registry:

```sh
herdr plugin install lmilojevicc/herdr-tab-rename
```

Local development:

```sh
go build -o herdr-tab-rename .
herdr plugin link .
```

> `herdr plugin link` registers the plugin from the current directory but does **not** build it — run `go build` first so the `herdr-tab-rename` binary exists.

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
