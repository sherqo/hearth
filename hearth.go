// Hearth — warm home energy (Go TUI, stdlib only).
// Battery status, power profile switch, system stats.
// Keys: up/down or j/k move · enter select · r refresh · q/esc quit.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Pink Cat Boo
const (
	cReset  = "\x1b[0m"
	cFg     = "\x1b[38;2;255;240;245m"
	cAccent = "\x1b[38;2;255;76;122m"
	cMuted  = "\x1b[38;2;86;89;112m"
	cDim    = "\x1b[38;2;120;124;150m"
	cGreen  = "\x1b[38;2;59;192;137m"
	cBold   = "\x1b[1m"
	altOn   = "\x1b[?1049h\x1b[H"
	altOff  = "\x1b[?1049l"
	hideCur = "\x1b[?25l"
	showCur = "\x1b[?25h"
	clear   = "\x1b[H\x1b[2J"
)

type Row struct {
	Kind   string // INFO, PROFILE, ACTION
	Text   string
	Value  string
	Action string
}

type State struct {
	Battery  string
	Active   string
	Profiles []string
	Stats    []string
	Message  string
}

func sh(args ...string) string {
	out, _ := exec.Command(args[0], args[1:]...).Output()
	return string(out)
}

func snapshot() State {
	var st State
	st.Battery = strings.TrimSpace(sh("omarchy-battery-status"))
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
	return st
}

func bar(pct int) string {
	const w = 16
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	n := pct * w / 100
	return strings.Repeat("█", n) + strings.Repeat("░", w-n)
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

func render(st *State, cur int) string {
	var b strings.Builder
	b.WriteString(clear)
	b.WriteString(cBold + cFg + "  Hearth" + cReset + cDim + "  ·  enter select · r refresh · q quit" + cReset + "\n\n")
	p := battPct(st.Battery)
	b.WriteString(fmt.Sprintf("  %s%s%s  %s%d%%%s\n\n", cFg, bar(p), cReset, cFg, p, cReset))
	b.WriteString("  " + cFg + st.Battery + cReset + "\n\n")

	rows := allRows(st)
	ord := map[int]int{} // allRows index -> selectable ordinal (-1 if not selectable)
	sel := selectable(st)
	_ = sel
	oi := 0
	for i, r := range rows {
		if r.Kind == "PROFILE" {
			ord[i] = oi
			oi++
		}
	}
	for i, r := range rows {
		prefix := "  "
		if o, ok := ord[i]; ok && o == cur {
			prefix = cAccent + "▸ " + cReset
		}
		switch r.Kind {
		case "HEAD":
			b.WriteString("\n" + cDim + "  " + r.Text + cReset + "\n")
		case "STAT":
			b.WriteString("  " + cDim + r.Text + cReset + "\n")
		case "PROFILE":
			mark := "  "
			if r.Value == st.Active {
				mark = cGreen + "● " + cReset
			}
			b.WriteString(prefix + mark + cFg + r.Text + cReset + "\n")
		}
	}
	if st.Message != "" {
		b.WriteString("\n" + cDim + "  " + st.Message + cReset + "\n")
	}
	return b.String()
}

func allRows(st *State) []Row {
	var rows []Row
	rows = append(rows, Row{"HEAD", "POWER PROFILE", "", ""})
	for _, p := range st.Profiles {
		rows = append(rows, Row{"PROFILE", p, p, "profile"})
	}
	rows = append(rows, Row{"HEAD", "SYSTEM", "", ""})
	for _, s := range st.Stats {
		rows = append(rows, Row{"STAT", s, "", ""})
	}
	return rows
}

func selectable(st *State) []Row {
	var rows []Row
	for _, r := range allRows(st) {
		if r.Kind == "PROFILE" {
			rows = append(rows, r)
		}
	}
	return rows
}

var tty *os.File

func rawOn() {
	tty, _ = os.OpenFile("/dev/tty", os.O_RDWR, 0)
	exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1", "-echo").Run()
	fmt.Print(altOn + hideCur)
}

func rawOff() {
	fmt.Print(showCur + altOff)
	exec.Command("stty", "-F", "/dev/tty", "sane").Run()
	if tty != nil {
		tty.Close()
	}
}

func readKey() string {
	buf := make([]byte, 8)
	n, _ := tty.Read(buf)
	if n == 0 {
		return ""
	}
	if buf[0] == 0x1b {
		if n == 1 {
			return "esc"
		}
		switch string(buf[1:n]) {
		case "[A":
			return "up"
		case "[B":
			return "down"
		}
		return "esc"
	}
	switch buf[0] {
	case 'q', 'Q':
		return "quit"
	case 'r', 'R':
		return "refresh"
	case '\r', '\n':
		return "enter"
	case 'j':
		return "down"
	case 'k':
		return "up"
	}
	return ""
}

func main() {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		os.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	}
	if len(os.Args) > 1 && os.Args[1] == "--dump" {
		st := snapshot()
		fmt.Printf("battery=%s active=%s profiles=%v\n", st.Battery, st.Active, st.Profiles)
		for _, s := range st.Stats {
			fmt.Printf("stat\t%s\n", s)
		}
		return
	}
	st := snapshot()
	rows := selectable(&st)
	cur := 0
	rawOn()
	defer rawOff()
	fmt.Print(render(&st, cur))

	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	keych := make(chan string, 8)
	go func() {
		for {
			keych <- readKey()
		}
	}()

	refresh := func() {
		msg := st.Message
		st = snapshot()
		st.Message = msg
		rows = selectable(&st)
		if cur >= len(rows) && len(rows) > 0 {
			cur = len(rows) - 1
		}
	}

	for {
		select {
		case k := <-keych:
			switch k {
			case "quit", "esc":
				return
			case "refresh":
				st.Message = ""
				refresh()
			case "up":
				if cur > 0 {
					cur--
				}
			case "down":
				if cur < len(rows)-1 {
					cur++
				}
			case "enter":
				if cur < len(rows) {
					st.Message = sh("omarchy-powerprofiles-set", "autodetect", rows[cur].Value)
					refresh()
				}
			}
			fmt.Print(render(&st, cur))
		case <-tick.C:
			msg := st.Message
			refresh()
			st.Message = msg
			fmt.Print(render(&st, cur))
		}
	}
}
