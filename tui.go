package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

type app struct {
	cfg           Config
	servers       []ServerState
	agents        []AgentState
	workflows     WorkflowFile
	focus         int
	serverSel     int
	agentSel      int
	workflowSel   int
	adding        bool
	input         string
	message       string
	lastRefresh   time.Time
	refreshActive bool
	pageIndex     int
	pagePaused    bool
	pageChangedAt time.Time
}

type pageDefinition struct {
	name  string
	focus int
	draw  func(*app, tcell.Screen, int, int)
}

var pageDefinitions = []pageDefinition{
	{name: "OVERVIEW", focus: -1, draw: drawOverviewPage},
	{name: "FLEET", focus: 1, draw: drawFleetPage},
}

var palette = struct {
	bg, panel, border, muted, text, green, yellow, red, cyan, purple tcell.Color
}{
	bg: tcell.NewHexColor(0x000000), panel: tcell.NewHexColor(0x080b10),
	border: tcell.NewHexColor(0x5f87af), muted: tcell.NewHexColor(0x8391a7),
	text: tcell.NewHexColor(0xf2f4f8), green: tcell.NewHexColor(0x00ff87),
	yellow: tcell.NewHexColor(0xffd75f), red: tcell.NewHexColor(0xff5f87),
	cyan: tcell.NewHexColor(0x00d7ff), purple: tcell.NewHexColor(0xaf87ff),
}

func newApp(cfg Config) *app {
	return &app{cfg: cfg, workflows: loadWorkflows(cfg.WorkflowsFile), pageChangedAt: time.Now()}
}

func (a *app) setPage(index int, now time.Time) {
	if len(pageDefinitions) == 0 {
		return
	}
	a.pageIndex = (index%len(pageDefinitions) + len(pageDefinitions)) % len(pageDefinitions)
	if pageDefinitions[a.pageIndex].focus >= 0 {
		a.focus = pageDefinitions[a.pageIndex].focus
	}
	a.pageChangedAt = now
}

func (a *app) maybeRotatePage(now time.Time) bool {
	if a.pagePaused || a.adding || len(pageDefinitions) < 2 {
		return false
	}
	interval := time.Duration(a.cfg.PageSeconds) * time.Second
	if now.Sub(a.pageChangedAt) < interval {
		return false
	}
	a.setPage(a.pageIndex+1, now)
	return true
}

func (a *app) pageRemaining(now time.Time) int {
	remaining := time.Duration(a.cfg.PageSeconds)*time.Second - now.Sub(a.pageChangedAt)
	if remaining <= 0 {
		return 0
	}
	return int((remaining + time.Second - 1) / time.Second)
}

func (a *app) refresh(ctx context.Context) {
	type result struct {
		i int
		s ServerState
	}
	ch := make(chan result, len(a.cfg.Servers))
	for i, cfg := range a.cfg.Servers {
		go func(i int, cfg ServerConfig) { ch <- result{i, collectServer(ctx, cfg)} }(i, cfg)
	}
	old := map[string]ServerState{}
	for _, s := range a.servers {
		old[s.Config.ID] = s
	}
	states := make([]ServerState, len(a.cfg.Servers))
	for range a.cfg.Servers {
		r := <-ch
		prev := old[r.s.Config.ID]
		r.s.CPUHistory = append(append([]float64{}, prev.CPUHistory...), r.s.Probe.CPUPercent)
		r.s.MemHistory = append(append([]float64{}, prev.MemHistory...), r.s.Probe.MemoryPercent)
		if len(r.s.CPUHistory) > 48 {
			r.s.CPUHistory = r.s.CPUHistory[len(r.s.CPUHistory)-48:]
			r.s.MemHistory = r.s.MemHistory[len(r.s.MemHistory)-48:]
		}
		states[r.i] = r.s
	}
	a.servers = states
	a.agents = loadAgents(a.cfg.AgentsDir)
	a.workflows = loadWorkflows(a.cfg.WorkflowsFile)
	a.lastRefresh = time.Now()
}

func (a *app) handleKey(e *tcell.EventKey) bool {
	a.pageChangedAt = time.Now()
	if a.adding {
		switch e.Key() {
		case tcell.KeyEscape:
			a.adding, a.input = false, ""
		case tcell.KeyEnter:
			title := strings.TrimSpace(a.input)
			if title != "" {
				a.workflows.Items = append(a.workflows.Items, WorkflowItem{ID: fmt.Sprintf("task-%d", time.Now().UnixNano()), Title: title, Status: "queued", Created: time.Now().UTC().Format(time.RFC3339)})
				a.workflowSel = len(a.workflows.Items) - 1
				a.persistWorkflows()
			}
			a.adding, a.input = false, ""
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if len(a.input) > 0 {
				_, n := utf8.DecodeLastRuneInString(a.input)
				a.input = a.input[:len(a.input)-n]
			}
		case tcell.KeyRune:
			a.input += string(e.Rune())
		}
		return false
	}

	if e.Key() == tcell.KeyCtrlC || e.Rune() == 'q' {
		return true
	}
	switch e.Key() {
	case tcell.KeyTAB:
		a.focus = (a.focus + 1) % 3
	case tcell.KeyBacktab:
		a.focus = (a.focus + 2) % 3
	case tcell.KeyUp:
		a.move(-1)
	case tcell.KeyDown:
		a.move(1)
	case tcell.KeyLeft:
		a.setPage(a.pageIndex-1, time.Now())
	case tcell.KeyRight:
		a.setPage(a.pageIndex+1, time.Now())
	}
	switch e.Rune() {
	case 'j':
		a.move(1)
	case 'k':
		a.move(-1)
	case '[':
		a.setPage(a.pageIndex-1, time.Now())
	case ']':
		a.setPage(a.pageIndex+1, time.Now())
	case 'p':
		a.pagePaused = !a.pagePaused
		a.message = "page rotation resumed"
		if a.pagePaused {
			a.message = "page rotation paused"
		}
	case 'a':
		if a.focus == 2 {
			a.adding = true
			a.input = ""
		}
	case ' ':
		if a.focus == 2 && len(a.workflows.Items) > 0 {
			i := clamp(a.workflowSel, 0, len(a.workflows.Items)-1)
			a.workflows.Items[i].Done = !a.workflows.Items[i].Done
			if a.workflows.Items[i].Done {
				a.workflows.Items[i].Status = "done"
			} else {
				a.workflows.Items[i].Status = "queued"
			}
			a.persistWorkflows()
		}
	}
	return false
}

func (a *app) persistWorkflows() {
	if err := saveWorkflows(a.cfg.WorkflowsFile, a.workflows); err != nil {
		a.message = "save failed: " + err.Error()
	} else {
		a.message = "workflow saved"
	}
}

func (a *app) move(delta int) {
	switch a.focus {
	case 0:
		a.agentSel = clamp(a.agentSel+delta, 0, max(0, len(a.agents)-1))
	case 1:
		a.serverSel = clamp(a.serverSel+delta, 0, max(0, len(a.servers)-1))
	case 2:
		a.workflowSel = clamp(a.workflowSel+delta, 0, max(0, len(a.workflows.Items)-1))
	}
}

func (a *app) draw(s tcell.Screen) {
	s.Clear()
	w, h := s.Size()
	base := tcell.StyleDefault.Background(palette.bg).Foreground(palette.text)
	fill(s, 0, 0, w, h, ' ', base)
	if w < 72 || h < 24 {
		put(s, 2, 1, "OPSDECK", base.Foreground(palette.cyan).Bold(true), w-4)
		put(s, 2, 3, "Terminal too small — use at least 72×24", base.Foreground(palette.yellow), w-4)
		s.Show()
		return
	}

	drawHeader(s, w, a)
	contentY, contentH := 2, h-4
	pageDefinitions[a.pageIndex].draw(a, s, contentY, contentH)
	drawFooter(s, w, h, a)
	s.Show()
}

func drawOverviewPage(a *app, s tcell.Screen, contentY, contentH int) {
	w, _ := s.Size()
	if w >= 108 {
		rightW := clamp(w/3, 32, 44)
		leftW := w - rightW - 1
		agentH := clamp(contentH*36/100, 9, contentH-12)
		if len(a.agents) == 0 {
			agentH = 7
		}
		drawBox(s, 0, contentY, leftW, agentH, " AI AGENTS ", a.focus == 0)
		a.drawAgents(s, 1, contentY+1, leftW-2, agentH-2)
		drawBox(s, 0, contentY+agentH, leftW, contentH-agentH, " SERVERS ", a.focus == 1)
		a.drawServers(s, 1, contentY+agentH+1, leftW-2, contentH-agentH-2)
		drawBox(s, leftW, contentY, rightW+1, contentH, " WORKFLOWS ", a.focus == 2)
		a.drawWorkflows(s, leftW+1, contentY+1, rightW-1, contentH-2)
	} else {
		agentH := max(7, contentH/4)
		if len(a.agents) == 0 {
			agentH = 3
		}
		workflowH := 7
		if contentH < 18 {
			workflowH = 5
		}
		serverH := contentH - agentH - workflowH
		drawBox(s, 0, contentY, w, agentH, " AI AGENTS ", a.focus == 0)
		a.drawAgents(s, 1, contentY+1, w-2, agentH-2)
		drawBox(s, 0, contentY+agentH, w, serverH, " SERVERS ", a.focus == 1)
		a.drawServers(s, 1, contentY+agentH+1, w-2, serverH-2)
		drawBox(s, 0, contentY+agentH+serverH, w, workflowH, " WORKFLOWS ", a.focus == 2)
		a.drawWorkflows(s, 1, contentY+agentH+serverH+1, w-2, workflowH-2)
	}
}

func drawFleetPage(a *app, s tcell.Screen, contentY, contentH int) {
	w, _ := s.Size()
	drawBox(s, 0, contentY, w, contentH, " SERVER FLEET ", true)
	a.drawServers(s, 1, contentY+1, w-2, contentH-2)
}

func drawHeader(s tcell.Screen, w int, a *app) {
	style := tcell.StyleDefault.Background(palette.bg).Foreground(palette.text)
	put(s, 1, 0, " OPSDECK ", style.Background(palette.cyan).Foreground(palette.bg).Bold(true), 11)
	online := 0
	for _, server := range a.servers {
		if server.Online {
			online++
		}
	}
	clock := time.Now().Format("Mon 15:04:05")
	rotation := fmt.Sprintf("%ds", a.pageRemaining(time.Now()))
	if a.pagePaused {
		rotation = "PAUSED"
	}
	page := pageDefinitions[a.pageIndex]
	pageLabel := fmt.Sprintf("%d/%d %s %s", a.pageIndex+1, len(pageDefinitions), page.name, rotation)
	pageX := w - len(clock) - len(pageLabel) - 5
	put(s, 14, 0, fmt.Sprintf("%d/%d servers  |  %d agents", online, len(a.servers), len(a.agents)), style.Foreground(palette.muted), max(0, pageX-15))
	put(s, pageX, 0, pageLabel, style.Foreground(palette.cyan), len(pageLabel))
	put(s, w-len(clock)-2, 0, clock, style.Foreground(palette.purple), len(clock))
}

func drawFooter(s tcell.Screen, w, h int, a *app) {
	style := tcell.StyleDefault.Background(palette.bg).Foreground(palette.muted)
	hint := "[left/right] page  [p] pause  [tab] panel  [j/k] move  [space] toggle  [a] add workflow  [q] quit"
	if w < 120 {
		hint = "[←/→] page  [p] pause  [tab] panel  [j/k] move  [q] quit"
	}
	if a.adding {
		hint = "new workflow › " + a.input + "█   [enter] save  [esc] cancel"
	} else if a.message != "" {
		hint = a.message + "   •   " + hint
	}
	put(s, 1, h-1, hint, style, w-2)
}

func (a *app) drawAgents(s tcell.Screen, x, y, w, h int) {
	style := tcell.StyleDefault.Background(palette.bg).Foreground(palette.text)
	if len(a.agents) == 0 {
		if h >= 1 {
			put(s, x+2, y, "No agents reporting yet", style.Foreground(palette.muted), w-4)
		}
		if h >= 4 {
			put(s, x+2, y+2, "Agents appear here when they write heartbeat JSON into:", style.Foreground(palette.muted), w-4)
			put(s, x+2, y+3, a.cfg.AgentsDir, style.Foreground(palette.cyan), w-4)
		}
		return
	}
	cardW := max(24, w/max(1, len(a.agents)))
	for i, ag := range a.agents {
		cx := x + i*cardW
		if cx >= x+w {
			break
		}
		cw := min(cardW-1, x+w-cx)
		selected := a.focus == 0 && i == a.agentSel
		drawMiniBox(s, cx, y, cw, h, selected)
		statusColor := palette.green
		status := strings.ToUpper(ag.Status)
		if staleAgent(ag) {
			status, statusColor = "STALE", palette.yellow
		}
		put(s, cx+2, y+1, truncate(ag.Name, cw-4), style.Bold(true), cw-4)
		put(s, cx+2, y+2, "* "+status, style.Foreground(statusColor), cw-4)
		put(s, cx+2, y+4, truncate(ag.Task, cw-4), style.Foreground(palette.muted), cw-4)
		put(s, cx+2, y+max(5, h-2), truncate(ag.Model, cw-4), style.Foreground(palette.purple), cw-4)
	}
}

func (a *app) drawServers(s tcell.Screen, x, y, w, h int) {
	style := tcell.StyleDefault.Background(palette.bg).Foreground(palette.text)
	if len(a.servers) == 0 {
		put(s, x+2, y+1, "No servers configured", style.Foreground(palette.muted), w-4)
		return
	}
	cols := len(a.servers)
	if w/cols < 24 {
		cols = max(1, w/24)
	}
	rows := (len(a.servers) + cols - 1) / cols
	cardW, cardH := w/cols, max(8, h/rows)
	for i, server := range a.servers {
		col, row := i%cols, i/cols
		cx, cy := x+col*cardW, y+row*cardH
		cw, ch := min(cardW-1, x+w-cx), min(cardH, y+h-cy)
		selected := a.focus == 1 && i == a.serverSel
		drawMiniBox(s, cx, cy, cw, ch, selected)
		color, dot := palette.green, "●"
		if !server.Online {
			color, dot = palette.red, "×"
		}
		put(s, cx+2, cy+1, dot+" "+truncate(server.Config.Name, cw-6), style.Foreground(color).Bold(true), cw-4)
		if server.Config.Kind == "http" {
			put(s, cx+2, cy+3, fmt.Sprintf("HTTP %-3d  %4dms", server.HTTPStatus, server.Latency.Milliseconds()), style.Foreground(palette.muted), cw-4)
			put(s, cx+2, cy+5, "external health probe", style.Foreground(palette.muted), cw-4)
			continue
		}
		if !server.Online {
			put(s, cx+2, cy+3, truncate(server.Error, cw-4), style.Foreground(palette.red), cw-4)
			continue
		}
		barW := max(5, cw-13)
		if ch < 11 {
			put(s, cx+2, cy+2, truncate(fmt.Sprintf("%s / %s", server.Probe.OSName, server.Probe.Arch), cw-4), style.Foreground(palette.muted), cw-4)
			put(s, cx+2, cy+3, truncate(fmt.Sprintf("%dt R%s D%s U%s", server.Probe.CPUThreads, bytesCompact(server.Probe.MemoryTotal), bytesCompact(server.Probe.DiskTotal), uptime(server.Probe.UptimeSeconds)), cw-4), style.Foreground(palette.cyan), cw-4)
			put(s, cx+2, cy+4, "CPU "+bar(server.Probe.CPUPercent, barW), style.Foreground(metricColor(server.Probe.CPUPercent)), cw-4)
			put(s, cx+2, cy+5, "MEM "+bar(server.Probe.MemoryPercent, barW), style.Foreground(metricColor(server.Probe.MemoryPercent)), cw-4)
			put(s, cx+2, cy+6, "DSK "+bar(server.Probe.DiskPercent, barW), style.Foreground(metricColor(server.Probe.DiskPercent)), cw-4)
		} else {
			put(s, cx+2, cy+2, truncate(fmt.Sprintf("%s / %s", server.Probe.OSName, server.Probe.Arch), cw-4), style.Foreground(palette.muted), cw-4)
			put(s, cx+2, cy+3, truncate(server.Probe.CPUModel, cw-4), style.Foreground(palette.purple), cw-4)
			put(s, cx+2, cy+4, truncate(fmt.Sprintf("%d threads | %s RAM", server.Probe.CPUThreads, bytes(server.Probe.MemoryTotal)), cw-4), style.Foreground(palette.cyan), cw-4)
			put(s, cx+2, cy+5, truncate(fmt.Sprintf("%s disk | up %s", bytes(server.Probe.DiskTotal), uptime(server.Probe.UptimeSeconds)), cw-4), style.Foreground(palette.cyan), cw-4)
			put(s, cx+2, cy+7, "CPU "+bar(server.Probe.CPUPercent, barW), style.Foreground(metricColor(server.Probe.CPUPercent)), cw-4)
			put(s, cx+2, cy+8, "MEM "+bar(server.Probe.MemoryPercent, barW), style.Foreground(metricColor(server.Probe.MemoryPercent)), cw-4)
			put(s, cx+2, cy+9, "DSK "+bar(server.Probe.DiskPercent, barW), style.Foreground(metricColor(server.Probe.DiskPercent)), cw-4)
		}
		if ch >= 13 {
			put(s, cx+2, cy+11, "cpu "+spark(server.CPUHistory, max(4, cw-8)), style.Foreground(palette.cyan), cw-4)
		}
		if ch >= 14 {
			put(s, cx+2, cy+12, "mem "+spark(server.MemHistory, max(4, cw-8)), style.Foreground(palette.purple), cw-4)
		}
		if ch >= 15 {
			healthy := 0
			for _, c := range server.Probe.Containers {
				if c.Health != "unhealthy" {
					healthy++
				}
			}
			put(s, cx+2, cy+13, fmt.Sprintf("%d/%d containers", healthy, len(server.Probe.Containers)), style.Foreground(palette.muted), cw-4)
		}
		if ch >= 16 {
			put(s, cx+2, cy+14, fmt.Sprintf("tx %s | rx %s", rate(server.Probe.NetTxBytesSec), rate(server.Probe.NetRxBytesSec)), style.Foreground(palette.muted), cw-4)
		}
		if ch >= 19 && len(server.Probe.Containers) > 0 {
			put(s, cx+2, cy+16, "CONTAINERS", style.Foreground(palette.cyan).Bold(true), cw-4)
			limit := min(len(server.Probe.Containers), ch-18)
			for j := 0; j < limit; j++ {
				container := server.Probe.Containers[j]
				mark, containerColor := "●", palette.green
				if container.Health == "unhealthy" || strings.Contains(strings.ToLower(container.Status), "exited") {
					mark, containerColor = "×", palette.red
				}
				put(s, cx+2, cy+17+j, mark+" "+truncate(container.Name, cw-6), style.Foreground(containerColor), cw-4)
			}
		}
	}
}

func (a *app) drawWorkflows(s tcell.Screen, x, y, w, h int) {
	style := tcell.StyleDefault.Background(palette.bg).Foreground(palette.text)
	done := 0
	for _, item := range a.workflows.Items {
		if item.Done {
			done++
		}
	}
	put(s, x+1, y, fmt.Sprintf("%d/%d complete", done, len(a.workflows.Items)), style.Foreground(palette.muted), w-2)
	if len(a.workflows.Items) > 0 {
		put(s, x+1, y+1, bar(100*float64(done)/float64(len(a.workflows.Items)), max(4, w-5)), style.Foreground(palette.purple), w-2)
	}
	startY := y + 3
	for i, item := range a.workflows.Items {
		if startY+i >= y+h {
			break
		}
		selected := a.focus == 2 && i == a.workflowSel
		st := style
		if selected {
			st = st.Background(palette.border).Bold(true)
		}
		mark := "[ ]"
		if item.Done {
			mark = "[x]"
			st = st.Foreground(palette.muted)
		} else {
			st = st.Foreground(palette.text)
		}
		line := fmt.Sprintf(" %s %s", mark, item.Title)
		put(s, x, startY+i, pad(truncate(line, w), w), st, w)
	}
	if len(a.workflows.Items) == 0 {
		put(s, x+1, startY, "Nothing queued", style.Foreground(palette.muted), w-2)
		put(s, x+1, startY+2, "Press a to add a workflow", style.Foreground(palette.cyan), w-2)
	}
}

func drawBox(s tcell.Screen, x, y, w, h int, title string, active bool) {
	color := palette.border
	if active {
		color = palette.cyan
	}
	st := tcell.StyleDefault.Background(palette.bg).Foreground(color)
	for i := x + 1; i < x+w-1; i++ {
		s.SetContent(i, y, '─', nil, st)
		s.SetContent(i, y+h-1, '─', nil, st)
	}
	for i := y + 1; i < y+h-1; i++ {
		s.SetContent(x, i, '│', nil, st)
		s.SetContent(x+w-1, i, '│', nil, st)
	}
	s.SetContent(x, y, '┌', nil, st)
	s.SetContent(x+w-1, y, '┐', nil, st)
	s.SetContent(x, y+h-1, '└', nil, st)
	s.SetContent(x+w-1, y+h-1, '┘', nil, st)
	put(s, x+2, y, title, st.Bold(true), w-4)
}

func drawMiniBox(s tcell.Screen, x, y, w, h int, active bool) {
	if w < 4 || h < 3 {
		return
	}
	color := palette.border
	if active {
		color = palette.purple
	}
	st := tcell.StyleDefault.Background(palette.bg).Foreground(color)
	for i := x + 1; i < x+w-1; i++ {
		s.SetContent(i, y, '─', nil, st)
		s.SetContent(i, y+h-1, '─', nil, st)
	}
	for i := y + 1; i < y+h-1; i++ {
		s.SetContent(x, i, '│', nil, st)
		s.SetContent(x+w-1, i, '│', nil, st)
	}
	s.SetContent(x, y, '┌', nil, st)
	s.SetContent(x+w-1, y, '┐', nil, st)
	s.SetContent(x, y+h-1, '└', nil, st)
	s.SetContent(x+w-1, y+h-1, '┘', nil, st)
}

func metricColor(v float64) tcell.Color {
	if v >= 90 {
		return palette.red
	}
	if v >= 70 {
		return palette.yellow
	}
	return palette.green
}

func bar(v float64, width int) string {
	width = max(1, width)
	v = min(100, maxFloat(0, v))
	filled := int(v / 100 * float64(width))
	return strings.Repeat("█", filled) + strings.Repeat("─", width-filled) + fmt.Sprintf(" %3.0f%%", v)
}

func spark(values []float64, width int) string {
	chars := []rune(" .:-=+*#%@")
	if width < 1 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	out := strings.Repeat(" ", width-len(values))
	for _, v := range values {
		i := int(maxFloat(0, min(100, v)) / 100 * float64(len(chars)-1))
		out += string(chars[i])
	}
	return out
}

func bytes(v uint64) string {
	units := []string{"B", "K", "M", "G", "T"}
	f := float64(v)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f%s", f, units[i])
}

func bytesCompact(v uint64) string {
	units := []string{"B", "K", "M", "G", "T"}
	f := float64(v)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.0f%s", f, units[i])
}

func uptime(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	days := int(d.Hours()) / 24
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, int(d.Hours())%24)
	}
	if d.Hours() >= 1 {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func rate(v float64) string {
	units := []string{"B", "K", "M", "G"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f%s/s", v, units[i])
}

func put(s tcell.Screen, x, y int, text string, style tcell.Style, limit int) {
	if limit <= 0 {
		return
	}
	i := 0
	for _, r := range text {
		if i >= limit {
			break
		}
		s.SetContent(x+i, y, r, nil, style)
		i++
	}
}

func fill(s tcell.Screen, x, y, w, h int, r rune, style tcell.Style) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			s.SetContent(xx, yy, r, nil, style)
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:max(0, n)])
	}
	return string(r[:n-1]) + "…"
}

func pad(s string, n int) string {
	l := len([]rune(s))
	if l >= n {
		return s
	}
	return s + strings.Repeat(" ", n-l)
}

func clamp(v, lo, hi int) int { return min(hi, max(lo, v)) }
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (a *app) snapshot(w, h int) error {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		return err
	}
	defer s.Fini()
	s.SetSize(w, h)
	a.draw(s)
	cells, gotW, gotH := s.GetContents()
	for y := 0; y < gotH; y++ {
		var line strings.Builder
		for x := 0; x < gotW; x++ {
			cell := cells[y*gotW+x]
			if len(cell.Runes) == 0 {
				line.WriteRune(' ')
			} else {
				line.WriteRune(cell.Runes[0])
			}
		}
		fmt.Fprintln(os.Stdout, strings.TrimRight(line.String(), " "))
	}
	return nil
}
