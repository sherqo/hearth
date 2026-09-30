# Hearth — warm home energy

Battery status, power profile switch, system stats.
Pink Cat Boo, CaskaydiaMono Nerd Font, zero idle RAM (exits on quit).

## Run
`hearth` (Go binary; Waybar battery click opens it floating).

## Build
`go build -o bin/hearth hearth.go` (stdlib only, no modules).

## Layout
- `hearth.go` — the TUI (self-prepends its `bin/` to PATH for helpers).
- `bin/` — backend: all Omarchy power helpers (`omarchy-battery-status`,
  `omarchy-powerprofiles-list/set`, `omarchy-system-stats`,
  `omarchy-cmd-present`).
- `waybar/` — module snippet + Hyprland float rule.

## Controls
`↑↓/jk` move · `enter` set profile · `r` refresh · `q/esc` quit.
