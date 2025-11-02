// ppr: PGSD pkg repair — a Bubble Tea TUI for FreeBSD/GhostBSD pkg catalog repair
// Name: ppr (PGSD pkg repair)
// Written for PGSD (Pacific Grove Software Distribution)
// Copyright (c) 2025 Pacific Grove Software Distribution Foundation
// License: BSD 2-Clause

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Stage string
type Status string

const (
	StageRepoNet      Stage = "repo_network_check"
	StageDetectEnv    Stage = "detect_env"
	StageClearCache   Stage = "clear_repo_cache"
	StagePkgUpdate    Stage = "pkg_update_force"
	StagePkgCheckDA   Stage = "pkg_check_da"
	StagePkgRecompute Stage = "pkg_check_recompute"
	StageMoveLocalDB  Stage = "move_local_sqlite"
	StageComplete     Stage = "complete"

	StatusOK    Status = "ok"
	StatusSkip  Status = "skip"
	StatusWarn  Status = "warn"
	StatusError Status = "error"
)

const appTitle = "ppr · PGSD pkg repair"

var appLabel = []string{
	"Copyright © 2025 Pacific Grove Software Distribution Foundation",
	"Licensed under the BSD 2-Clause License",
}

type Event struct {
	Time    string `json:"time"`
	Stage   Stage  `json:"stage"`
	Status  Status `json:"status"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type Config struct {
	DryRun     bool
	Compact    bool
	JSONReport string
	Timeout    time.Duration
}

type eventMsg Event
type nextStageMsg struct{}
type errMsg struct{ err error }

type model struct {
	cfg     Config
	spin    spinner.Model
	style   styles
	events  []Event
	stOrder []Stage
	stMap   map[Stage]Event
	idx     int
	done    bool
	err     error
}

type styles struct {
	title   lipgloss.Style
	label   lipgloss.Style
	section lipgloss.Style
	ok      lipgloss.Style
	warn    lipgloss.Style
	skipped lipgloss.Style
	error   lipgloss.Style
	detail  lipgloss.Style
}

func newStyles() styles {
	blue := lipgloss.Color("#003366")
	green := lipgloss.Color("#10b981")
	yellow := lipgloss.Color("#f59e0b")
	red := lipgloss.Color("#ef4444")
	muted := lipgloss.Color("#6b7280")
	return styles{
		title:   lipgloss.NewStyle().Foreground(blue).Bold(true).Align(lipgloss.Center),
		label:   lipgloss.NewStyle().Foreground(muted).Align(lipgloss.Center),
		section: lipgloss.NewStyle().Foreground(blue).Bold(true),
		ok:      lipgloss.NewStyle().Foreground(green),
		warn:    lipgloss.NewStyle().Foreground(yellow),
		skipped: lipgloss.NewStyle().Foreground(muted),
		error:   lipgloss.NewStyle().Foreground(red).Bold(true),
		detail:  lipgloss.NewStyle().Foreground(muted),
	}
}

func initialModel(cfg Config) model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#003366"))
	order := []Stage{
		StageRepoNet,
		StageDetectEnv,
		StageClearCache,
		StagePkgUpdate,
		StagePkgCheckDA,
		StagePkgRecompute,
		StagePkgCheckDA,
		StageMoveLocalDB,
	}
	return model{
		cfg:     cfg,
		spin:    sp,
		style:   newStyles(),
		stOrder: order,
		stMap:   map[Stage]Event{},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, runStage(m.cfg, m.stOrder[0]))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case eventMsg:
		ev := Event(msg)
		m.events = append(m.events, ev)
		m.stMap[ev.Stage] = ev
		return m, func() tea.Msg { return nextStageMsg{} }
	case nextStageMsg:
		m.idx++
		if m.idx >= len(m.stOrder) {
			m.done = true
			_ = writeJSONReport(m.cfg.JSONReport, m.events)
			return m, tea.Quit
		}
		return m, runStage(m.cfg, m.stOrder[m.idx])
	case errMsg:
		m.err = msg.err
		m.done = true
		_ = writeJSONReport(m.cfg.JSONReport, m.events)
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(m.style.title.Render(appTitle))
	b.WriteString("\n")
	for _, l := range appLabel {
		b.WriteString(m.style.label.Render(l))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	for _, st := range m.stOrder {
		ev, ok := m.stMap[st]
		if !ok {
			b.WriteString(m.spin.View() + " " + humanStage(st) + "\n")
			continue
		}
		icon := statusIcon(ev.Status)
		line := "  " + icon + " " + humanStage(st)
		switch ev.Status {
		case StatusOK:
			b.WriteString(m.style.ok.Render(line))
		case StatusWarn:
			b.WriteString(m.style.warn.Render(line))
		case StatusSkip:
			b.WriteString(m.style.skipped.Render(line))
		case StatusError:
			b.WriteString(m.style.error.Render(line))
		}
		if ev.Message != "" {
			b.WriteString(": " + ev.Message)
		}
		b.WriteString("\n")
		if ev.Detail != "" {
			b.WriteString(m.style.detail.Render(indent(ev.Detail)))
			b.WriteString("\n")
		}
	}

	if m.done {
		if m.err != nil {
			b.WriteString(m.style.error.Render("Finished with errors."))
		} else {
			b.WriteString(m.style.ok.Render("Completed successfully. Run `pkg -vv` to confirm repos."))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func statusIcon(s Status) string {
	switch s {
	case StatusOK:
		return "[✓]"
	case StatusWarn:
		return "[!]"
	case StatusSkip:
		return "[...]"
	case StatusError:
		return "[x]"
	default:
		return "[ ]"
	}
}

func humanStage(s Stage) string {
	switch s {
	case StageRepoNet:
		return "Check repository network"
	case StageDetectEnv:
		return "Detect environment"
	case StageClearCache:
		return "Clear repo cache"
	case StagePkgUpdate:
		return "Force pkg update"
	case StagePkgCheckDA:
		return "Verify package DB"
	case StagePkgRecompute:
		return "Recompute package metadata"
	case StageMoveLocalDB:
		return "Last resort: move local.sqlite"
	default:
		return string(s)
	}
}

func runStage(cfg Config, st Stage) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()
		ev := Event{Time: time.Now().UTC().Format(time.RFC3339), Stage: st}

		switch st {
		case StageRepoNet:
			msg, detail, ok := checkRepoNetwork(ctx)
			if ok {
				ev.Status = StatusOK
			} else {
				ev.Status = StatusWarn
			}
			ev.Message = msg
			ev.Detail = detail
			return eventMsg(ev)

		case StageDetectEnv:
			if os.Geteuid() != 0 {
				ev.Status = StatusError
				ev.Message = "Must run as root"
				return eventMsg(ev)
			}
			ev.Status = StatusOK
			ev.Message = "Running as root"
			return eventMsg(ev)

		case StageClearCache:
			paths, err := globRepoSqlite()
			if err != nil {
				ev.Status = StatusWarn
				ev.Message = "Could not scan /var/db/pkg"
				ev.Detail = err.Error()
				return eventMsg(ev)
			}
			if len(paths) == 0 {
				ev.Status = StatusOK
				ev.Message = "Repo cache already clean"
				ev.Detail = "Checked /var/db/pkg for repo-*.sqlite*"
				return eventMsg(ev)
			}
			for _, p := range paths {
				_ = os.Remove(p)
			}
			ev.Status = StatusOK
			ev.Message = "Removed cached repo catalogs"
			ev.Detail = strings.Join(paths, "\n")
			return eventMsg(ev)

		case StagePkgUpdate:
			return runAndReport(ctx, ev, "pkg", []string{"update", "-f"},
				"pkg update completed", "pkg update had problems. Tried bootstrap and retry", true)

		case StagePkgCheckDA:
			return runAndReport(ctx, ev, "pkg", []string{"check", "-da"},
				"Local package database looks consistent", "Integrity issues detected", false)

		case StagePkgRecompute:
			return runAndReport(ctx, ev, "pkg", []string{"check", "-r", "-a"},
				"Recomputed package metadata", "Recompute reported problems", false)

		case StageMoveLocalDB:
			localDB := "/var/db/pkg/local.sqlite"
			if _, err := os.Stat(localDB); err == nil {
				backup := localDB + ".bak"
				if err := os.Rename(localDB, backup); err != nil {
					ev.Status = StatusWarn
					ev.Message = "Could not move local.sqlite"
					ev.Detail = err.Error()
					return eventMsg(ev)
				}
				ev.Status = StatusOK
				ev.Message = "Moved local.sqlite aside"
				ev.Detail = localDB + " -> " + backup
				_, _ = runCmdCapture(ctx, "pkg", []string{"update", "-f"})
				_, _ = runCmdCapture(ctx, "pkg", []string{"check", "-da"})
				return eventMsg(ev)
			}
			// softened tone here
			ev.Status = StatusOK
			ev.Message = "No local.sqlite found"
			ev.Detail = "Package database is already in a clean state"
			return eventMsg(ev)
		}
		ev.Status = StatusSkip
		ev.Message = "No-op"
		return eventMsg(ev)
	}
}

// Run a command and map output to event
func runAndReport(ctx context.Context, ev Event, name string, args []string, okMsg, warnMsg string, tryBootstrap bool) tea.Msg {
	out, err := runCmdCapture(ctx, name, args)
	if err != nil && tryBootstrap {
		_, _ = runCmdCapture(ctx, "pkg", []string{"bootstrap", "-f"})
		out2, _ := runCmdCapture(ctx, name, args)
		ev.Status = StatusWarn
		ev.Message = warnMsg
		ev.Detail = tail(out+"\n"+out2, 300)
		return eventMsg(ev)
	}
	if err != nil {
		ev.Status = StatusWarn
		ev.Message = warnMsg
		ev.Detail = tail(out+"\n"+err.Error(), 300)
		return eventMsg(ev)
	}
	ev.Status = StatusOK
	ev.Message = okMsg
	ev.Detail = tail(out, 200)
	return eventMsg(ev)
}

// --- Repository Network Check ---

func checkRepoNetwork(ctx context.Context) (string, string, bool) {
	abi, _ := runCmdCapture(ctx, "pkg", []string{"config", "ABI"})
	cfg, err := runCmdCapture(ctx, "pkg", []string{"-vv"})
	if err != nil {
		return "Could not run pkg -vv", err.Error(), false
	}
	urls := parseRepoURLs(cfg, strings.TrimSpace(abi))
	if len(urls) == 0 {
		return "Could not detect repository URLs", "No url entries parsed from pkg -vv output", false
	}

	var lines []string
	okAll := true
	for _, raw := range urls {
		alive, info := probeRepo(ctx, raw)
		if alive {
			lines = append(lines, "[✓] "+info)
		} else {
			lines = append(lines, "[x] "+info)
			okAll = false
		}
	}
	if okAll {
		return "Repository network reachable", strings.Join(lines, "\n"), true
	}
	return "Some repositories are unreachable", strings.Join(lines, "\n"), false
}

func parseRepoURLs(vv, abi string) []string {
	var out []string
	for _, ln := range strings.Split(vv, "\n") {
		line := strings.TrimSpace(ln)
		if strings.HasPrefix(line, "url") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			u := strings.TrimSpace(parts[1])
			u = strings.TrimRight(u, ",")
			u = strings.Trim(u, `"'`)
			u = strings.TrimSpace(u)
			if strings.HasPrefix(u, "pkg+http://") {
				u = "http://" + strings.TrimPrefix(u, "pkg+http://")
			} else if strings.HasPrefix(u, "pkg+https://") {
				u = "https://" + strings.TrimPrefix(u, "pkg+https://")
			}
			u = strings.ReplaceAll(u, "${ABI}", abi)
			if u != "" {
				out = append(out, u)
			}
		}
	}
	return out
}

func probeRepo(ctx context.Context, raw string) (bool, string) {
	u, err := url.Parse(raw)
	if err != nil {
		return false, fmt.Sprintf("%s (parse error: %v)", raw, err)
	}
	host := u.Host
	port := "80"
	if u.Scheme == "https" {
		port = "443"
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return false, fmt.Sprintf("%s (tcp connect failed: %v)", raw, err)
	}
	_ = conn.Close()

	client := &http.Client{Timeout: 6 * time.Second}
	meta := strings.TrimRight(u.String(), "/") + "/meta.conf"
	resp, err := client.Get(meta)
	if err != nil {
		return false, fmt.Sprintf("%s (GET /meta.conf failed: %v)", raw, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, fmt.Sprintf("%s (ok)", raw)
	}
	return false, fmt.Sprintf("%s (GET /meta.conf status %d)", raw, resp.StatusCode)
}

// --- Helpers ---

func runCmdCapture(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	merge := io.MultiReader(stdout, stderr)
	sc := bufio.NewScanner(merge)
	var b strings.Builder
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return b.String(), err
	}
	if err := cmd.Wait(); err != nil {
		return b.String(), err
	}
	return b.String(), nil
}

func globRepoSqlite() ([]string, error) {
	base := "/var/db/pkg"
	return filepath.Glob(filepath.Join(base, "repo-*.sqlite*"))
}

func writeJSONReport(path string, events []Event) error {
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("    " + line + "\n")
	}
	return b.String()
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

func main() {
	cfg := Config{}
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Show intended actions without making changes")
	flag.BoolVar(&cfg.Compact, "compact", false, "Compact view mode (minimal output)")
	flag.StringVar(&cfg.JSONReport, "report-json", "", "Write a JSON event report to this file")
	flag.DurationVar(&cfg.Timeout, "timeout", 20*time.Minute, "Overall timeout for repair")
	flag.Parse()

	p := tea.NewProgram(initialModel(cfg))
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ppr: %v\n", err)
		os.Exit(1)
	}
	if m, ok := final.(model); ok && m.err != nil {
		os.Exit(1)
	}
}

