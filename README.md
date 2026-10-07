# Hearth — warm home energy

Battery status, charge-limit switch, power-profile switch, power hogs.
Pink Cat Boo, zero idle RAM (exits on quit).

## Run
`hearth` (Go binary; Waybar battery click opens it floating).

## Build
`go build -o hearth .` (Bubble Tea/Lipgloss, see `go.mod`),
or straight from the network with no clone:

```bash
go install github.com/sherqo/hearth@latest
```

## Install (Arch Linux)

Runtime deps (the backend shells out to them):

```bash
sudo pacman -S --needed upower power-profiles-daemon
```

Charge-limit changes need root; Hearth asks via `polkit` (`pkexec`),
which is part of base `polkit`. No Nerd Font needed — plain text UI.

The backend ships inside the binary: on first run Hearth extracts its
helpers to `~/.local/share/hearth` (or `$XDG_DATA_HOME/hearth`) and
re-syncs them whenever they change. No Makefile, no manual copying.

## Layout
- `hearth.go` — the TUI.
- `bin/omarchy-*` — power backend vendored from Omarchy (see `ATTRIBUTION.md`).
- `waybar/` — module snippet + Hyprland float rule.

## Controls
`↑↓/jk` move · `h/l` charge limit −/+5% · `enter` select ·
`x` kill hog (press twice to confirm) · `r` refresh · `q/esc` quit.

## License
MIT — see `LICENSE`. Omarchy-derived files keep their original terms;
see `ATTRIBUTION.md` and `LICENSE.omarchy`.
