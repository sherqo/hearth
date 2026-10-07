// Hearth — warm home energy (Bubble Tea TUI).
// Battery status, charge limit, power profiles, system stats, power hogs.
// Backend is Omarchy's power scripts, byte-identical (see bin/).
// Keys: up/down or j/k move · h/l move pill · enter select ·
// x kill hog (twice to confirm) · r refresh · q/esc quit.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Pink Cat Boo
var (
	cBg      = lipgloss.Color("#202330")
	cFg      = lipgloss.Color("#FFF0F5")
	cAccent  = lipgloss.Color("#FF4C7A")
	cMuted   = lipgloss.Color("#565970")
	cDim     = lipgloss.Color("#8A8DA3")
	cGreen   = lipgloss.Color("#3BC089")
	cBlue    = lipgloss.Color("#6767CE")
	cYellow  = lipgloss.Color("#FEC831")
	titleSt  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	nameSt   = lipgloss.NewStyle().Foreground(cFg)
	dimSt    = lipgloss.NewStyle().Foreground(cMuted)
	selSt    = lipgloss.NewStyle().Foreground(cFg).Background(lipgloss.Color("#3A3048")).Bold(true)
	headSt   = lipgloss.NewStyle().Foreground(cFg).Bold(true)
	boxSt    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cMuted).Padding(0, 2, 1, 2).Background(cBg)
	helpSt   = lipgloss.NewStyle().Foreground(cMuted)
	pillOn   = lipgloss.NewStyle().Foreground(cBg).Background(cAccent).Bold(true).Padding(0, 1)
	pillOff  = lipgloss.NewStyle().Foreground(cDim).Padding(0, 1)
	activeNm = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	mutSt    = lipgloss.NewStyle().Foreground(cYellow).Bold(true)
)

//go:embed bin/omarchy-*
var backendFS embed.FS

func backendRoot() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "hearth")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "hearth")
}

func ensureBackend() string {
	root := backendRoot()
	binDir := filepath.Join(root, "bin")
	_ = os.MkdirAll(binDir, 0o755)
	_ = fs.WalkDir(backendFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == "." {
			return nil
		}
		data, err := backendFS.ReadFile(path)
		if err != nil {
			return nil
		}
		dst := filepath.Join(root, path)
		if cur, err := os.ReadFile(dst); err == nil && string(cur) == string(data) {
			return nil
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, data, 0o755)
		return nil
	})
	return binDir
}

func hasNerdFont() bool {
	out, err := exec.Command("fc-list", ":", "family").Output()
	if err != nil {
		return true
	}
	return strings.Contains(strings.ToLower(string(out)), "nerd")
}

var useASCII = false

type Hog struct {
	Pid  string
	Comm string
	CPU  string
}

type Row struct {
	Kind  string // charge, profile, hog
	Text  string
	Value string
}

type State struct {
	Battery  string
	Pct      int
	Time     string
	Health   int
	Rate     string
	Active   string
	Profiles []string
	Stats    []string
	Limit    int
	Bat      string
	Hogs     []Hog
	Message  string
}

func sh(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	return string(out)
}

func batDir() string {
	entries, _ := os.ReadDir("/sys/class/power_supply")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "BAT") {
			return "/sys/class/power_supply/" + e.Name()
		}
	}
	return ""
}

func battPath() string {
	out := strings.TrimSpace(sh("upower", "-e"))
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "BAT") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func snapshot() State {
	var st State
	// structured backend output: percentage/state/rate/size/time/threshold
	kv := map[string]string{}
	for _, line := range strings.Split(sh("omarchy-battery-status", "--shell"), "\n") {
		p := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(p) == 2 {
			kv[p[0]] = p[1]
		}
	}
	st.Battery = strings.TrimSpace(sh("omarchy-battery-status"))
	if p, ok := kv["percentage"]; ok {
		st.Pct, _ = strconv.Atoi(strings.TrimSuffix(p, "%"))
	} else {
		st.Pct = battPct(st.Battery)
	}
	st.Time = kv["time"]
	st.Rate = kv["rate"]
	// health = upower capacity% (energy-full vs design)
	for _, line := range strings.Split(sh("upower", "-i", battPath()), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "capacity:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				if fv, err := strconv.ParseFloat(strings.TrimSuffix(f[1], "%"), 64); err == nil {
					st.Health = int(fv)
				}
			}
		}
	}
	for _, line := range strings.Split(sh("omarchy-powerprofiles-list"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "*") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
			st.Active = line
		}
		st.Profiles = append(st.Profiles, line)
	}
	if st.Active == "" {
		st.Active = strings.TrimSpace(sh("powerprofilesctl", "get"))
	}
	for _, line := range strings.Split(sh("omarchy-system-stats"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			st.Stats = append(st.Stats, line)
		}
	}
	if dir := batDir(); dir != "" {
		st.Bat = dir
		if raw, err := os.ReadFile(dir + "/charge_control_end_threshold"); err == nil {
			st.Limit, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
		}
	}
	// top CPU consumers (skip the header + our own ps/hearth)
	for _, line := range strings.Split(sh("ps", "-eo", "pid,comm,%cpu", "--sort=-%cpu"), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] == "PID" {
			continue
		}
		if f[1] == "ps" || f[1] == "hearth" {
			continue
		}
		st.Hogs = append(st.Hogs, Hog{f[0], f[1], f[2]})
		if len(st.Hogs) >= 6 {
			break
		}
	}
	return st
}

func battPct(s string) int {
	for _, f := range strings.Fields(s) {
		if strings.HasSuffix(f, "%") {
			n, _ := strconv.Atoi(strings.TrimSuffix(f, "%"))
			return n
		}
	}
	return 0
}

func shortName(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func bar(pct, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	n := pct * width / 100
	f := lipgloss.NewStyle().Foreground(cGreen).Render(strings.Repeat("━", n))
	e := lipgloss.NewStyle().Foreground(cMuted).Render(strings.Repeat("━", width-n))
	return f + e
}

func layoutWidths(termW int) (boxW, nameW int) {
	boxW = termW - 2
	if termW <= 0 {
		boxW = 70
	}
	if boxW < 48 {
		boxW = 48
	}
	if boxW > 100 {
		boxW = 100
	}
	nameW = boxW - 2 - 4 - 26
	if nameW < 14 {
		nameW = 14
	}
	if nameW > 28 {
		nameW = 28
	}
	return boxW, nameW
}

var chargeOptions = []int{60, 80, 100}

type model struct {
	st           State
	cursor       int
	chargeCursor int
	width        int
	height       int
	busy         bool
	flash        string
	armedKill    string
}

type refreshMsg State
type doneMsg string

func doSnapshot() tea.Msg { return refreshMsg(snapshot()) }

func tickRefresh() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return doSnapshot() })
}

func (m model) Init() tea.Cmd { return tickRefresh() }

func selsOf(st *State) []Row {
	var out []Row
	out = append(out, Row{"charge", "", ""})
	for _, p := range st.Profiles {
		out = append(out, Row{"profile", p, p})
	}
	for _, h := range st.Hogs {
		label := fmt.Sprintf("%-16.16s %6s%%  %s", h.Comm, h.CPU, h.Pid)
		out = append(out, Row{"hog", label, h.Pid})
	}
	return out
}

func clampCursor(m *model, n int) {
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
}

// contentLines mirrors View: charge row, profile section, hog section.
func contentLines(st *State) []int {
	var out []int
	idx := 0
	out = append(out, idx) // charge
	idx++
	out = append(out, -1, -1) // blank + POWER PROFILE
	for range st.Profiles {
		out = append(out, idx)
		idx++
	}
	out = append(out, -1, -1) // blank + POWER HOGS
	for range st.Hogs {
		out = append(out, idx)
		idx++
	}
	return out
}

func availH(termH int) int {
	if termH <= 0 {
		return 1 << 30
	}
	h := termH - 4 - 2
	if h < 3 {
		h = 3
	}
	return h
}

func windowStart(total, cursorPos, maxH int) int {
	if total <= maxH {
		return 0
	}
	s := cursorPos - maxH/2
	if s < 0 {
		s = 0
	}
	if s > total-maxH {
		s = total - maxH
	}
	return s
}

// rowAtY maps a terminal line through the same flat list View renders.
// Header occupies lines 1..3 (flash, blank, hero), content from 4.
func rowAtY(m *model, ss []Row, termW, termH, cursor, y int) int {
	lines := contentLines(&m.st)
	_ = ss
	_ = termW
	maxH := availH(termH)
	cpos := 0
	for pos, li := range lines {
		if li == cursor {
			cpos = pos
			break
		}
	}
	start := windowStart(len(lines), cpos, maxH)
	pos := start + (y - 4)
	if pos < start || pos >= start+maxH || pos < 0 || pos >= len(lines) {
		return -1
	}
	return lines[pos]
}

func setChargeLimit(limit int) string {
	dir := batDir()
	if dir == "" {
		return "no battery found"
	}
	target := filepath.Join(dir, "charge_control_end_threshold")
	want := strconv.Itoa(limit)
	if cur, err := os.ReadFile(target); err == nil && strings.TrimSpace(string(cur)) == want {
		return fmt.Sprintf("charge limit already %d%%", limit)
	}
	// pkexec pops a system auth dialog; never blocks the TUI on a tty prompt
	cmd := exec.Command("pkexec", "tee", target)
	cmd.Stdin = strings.NewReader(want)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "charge limit needs auth (" + shortName(strings.TrimSpace(string(out)), 40) + ")"
	}
	return fmt.Sprintf("charge limit → %d%%", limit)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case refreshMsg:
		incoming := State(msg)
		incoming.Message = m.st.Message
		m.st = incoming
		clampCursor(&m, len(selsOf(&m.st)))
		if m.chargeCursor > len(chargeOptions)-1 {
			m.chargeCursor = 0
		}
		return m, tickRefresh()
	case doneMsg:
		m.busy = false
		m.flash = shortName(string(msg), 60)
		return m, func() tea.Msg { return doSnapshot() }
	case tea.MouseMsg:
		ss := selsOf(&m.st)
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if i := rowAtY(&m, ss, m.width, m.height, m.cursor, msg.Y); i >= 0 {
				m.cursor = i
			} else if m.cursor > 0 {
				m.cursor--
			}
		case tea.MouseButtonWheelDown:
			if i := rowAtY(&m, ss, m.width, m.height, m.cursor, msg.Y); i >= 0 {
				m.cursor = i
			} else if m.cursor < len(ss)-1 {
				m.cursor++
			}
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress {
				if i := rowAtY(&m, ss, m.width, m.height, m.cursor, msg.Y); i >= 0 {
					m.cursor = i
				}
			}
		case tea.MouseButtonRight:
			if msg.Action == tea.MouseActionPress {
				m.armedKill = ""
			}
		}
		return m, nil
	case tea.KeyMsg:
		ss := selsOf(&m.st)
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.flash = ""
			m.armedKill = ""
			return m, func() tea.Msg { return doSnapshot() }
		case "up", "k":
			m.armedKill = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			m.armedKill = ""
			if m.cursor < len(ss)-1 {
				m.cursor++
			}
		case "left", "h":
			if m.cursor < len(ss) && ss[m.cursor].Kind == "charge" {
				m.armedKill = ""
				m.chargeCursor--
				if m.chargeCursor < 0 {
					m.chargeCursor = len(chargeOptions) - 1
				}
			}
		case "right", "l":
			if m.cursor < len(ss) && ss[m.cursor].Kind == "charge" {
				m.armedKill = ""
				m.chargeCursor++
				if m.chargeCursor >= len(chargeOptions) {
					m.chargeCursor = 0
				}
			}
		case "x":
			if m.cursor < len(ss) && ss[m.cursor].Kind == "hog" {
				pid := ss[m.cursor].Value
				if m.armedKill == pid {
					m.armedKill = ""
					m.busy = true
					return m, func() tea.Msg {
						if err := exec.Command("kill", pid).Run(); err != nil {
							return doneMsg("kill " + pid + " failed")
						}
						return doneMsg("killed " + pid)
					}
				}
				m.armedKill = pid
				m.flash = "x again to kill " + pid
			}
		case "enter":
			if m.cursor < len(ss) && !m.busy {
				r := ss[m.cursor]
				switch r.Kind {
				case "charge":
					limit := chargeOptions[m.chargeCursor]
					m.busy = true
					return m, func() tea.Msg { return doneMsg(setChargeLimit(limit)) }
				case "profile":
					m.busy = true
					return m, func() tea.Msg {
						return doneMsg(strings.TrimSpace(sh("omarchy-powerprofiles-set", "autodetect", r.Value)))
					}
				}
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	boxW, nameW := layoutWidths(m.width)
	var b strings.Builder
	flash := " "
	if m.flash != "" {
		flash = "  " + lipgloss.NewStyle().Foreground(cBlue).Render(m.flash)
	} else if m.busy {
		flash = "  " + lipgloss.NewStyle().Foreground(cYellow).Render("working…")
	}
	b.WriteString(flash + "\n")
	// hero: battery bar + tight metrics (Zephyr style: single-space · separators)
	p := m.st.Pct
	b.WriteString("  " + bar(p, 16) + "  " + nameSt.Render(fmt.Sprintf("%d%%", p)) + "\n")
	meta := ""
	if m.st.Time != "" {
		meta += " · " + m.st.Time
	}
	if m.st.Rate != "" {
		meta += " · " + m.st.Rate
	}
	if m.st.Health > 0 {
		meta += " · health " + strconv.Itoa(m.st.Health) + "%"
	}
	meta = strings.TrimPrefix(meta, " · ")
	b.WriteString("  " + dimSt.Render(meta) + "\n")
	ss := selsOf(&m.st)
	lines := contentLines(&m.st)
	maxH := availH(m.height)
	cpos := 0
	for pos, li := range lines {
		if li == m.cursor {
			cpos = pos
			break
		}
	}
	start := windowStart(len(lines), cpos, maxH)
	end := start + maxH
	if end > len(lines) {
		end = len(lines)
	}
	// render windowed rows with section headers
	lastKind := ""
	for _, li := range lines[start:end] {
		if li < 0 {
			continue
		}
		r := ss[li]
		if r.Kind != lastKind {
			switch r.Kind {
			case "charge":
				b.WriteString("\n" + headSt.Render("CHARGE LIMIT") + "\n")
			case "profile":
				if lastKind != "profile" {
					b.WriteString("\n" + headSt.Render("POWER PROFILE") + "\n")
				}
			case "hog":
				if lastKind != "hog" {
					b.WriteString("\n" + headSt.Render("POWER HOGS") + "\n")
				}
			}
			lastKind = r.Kind
		}
		var line string
		switch r.Kind {
		case "charge":
			var pills []string
			for bi, opt := range chargeOptions {
				label := fmt.Sprintf("%d%%", opt)
				if opt == m.st.Limit {
					label += " ●"
				}
				if bi == m.chargeCursor {
					pills = append(pills, pillOn.Render(label))
				} else {
					pills = append(pills, pillOff.Render(label))
				}
			}
			// custom limit (e.g. 70) not in presets: show it
			custom := true
			for _, opt := range chargeOptions {
				if opt == m.st.Limit {
					custom = false
				}
			}
			if custom && m.st.Limit > 0 {
				pills = append(pills, dimSt.Render(fmt.Sprintf("now %d%%", m.st.Limit)))
			}
			line = "  " + strings.Join(pills, " ")
		case "profile":
			mark := "  "
			if r.Value == m.st.Active {
				mark = lipgloss.NewStyle().Foreground(cGreen).Render("● ")
			}
			line = mark + nameSt.Render(r.Text)
		case "hog":
			line = "  " + dimSt.Render(r.Text)
		}
		if li == m.cursor {
			b.WriteString(selSt.Render("▸ "+line) + "\n")
		} else {
			b.WriteString(dimSt.Render("  ") + line + "\n")
		}
	}
	_ = nameW
	return boxSt.Width(boxW).Render(b.String())
}

func main() {
	binDir := ensureBackend()
	paths := []string{binDir}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Dir(exe))
	}
	os.Setenv("PATH", strings.Join(paths, ":")+":"+os.Getenv("PATH"))
	useASCII = !hasNerdFont() || os.Getenv("HEARTH_ASCII") == "1"
	if len(os.Args) > 1 && os.Args[1] == "--dump" {
		st := snapshot()
		fmt.Printf("battery=%s pct=%d time=%s rate=%s health=%d active=%s profiles=%v limit=%d\n", st.Battery, st.Pct, st.Time, st.Rate, st.Health, st.Active, st.Profiles, st.Limit)
		for _, h := range st.Hogs {
			fmt.Printf("hog\t%s\t%s\t%s\n", h.Pid, h.Comm, h.CPU)
		}
		return
	}
	m := model{st: snapshot()}
	// preselect charge pill matching current limit
	for i, opt := range chargeOptions {
		if opt == m.st.Limit {
			m.chargeCursor = i
			break
		}
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "hearth:", err)
		os.Exit(1)
	}
}
