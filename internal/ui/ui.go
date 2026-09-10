package ui

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haffi96/lazyiperf/internal/engine"
)

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
var border = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("60")).Padding(0, 1)

type field struct {
	name, help string
	choices    []string
}
type event struct {
	id int
	s  engine.Snapshot
}
type model struct {
	ctx           context.Context
	c             engine.Config
	original      engine.Config
	cancel        context.CancelFunc
	id            int
	events        chan event
	width, height int
	wizard        bool
	fields        []field
	step          int
	input, err    string
	s             engine.Snapshot
	logs          []string
	history       []float64
	offset        int
}

func Run(ctx context.Context, c engine.Config) error {
	if c.Target != "" {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	m := model{ctx: ctx, c: c, width: 100, height: 32}
	if c.Target == "" {
		m.openWizard("")
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
func (m model) Init() tea.Cmd {
	if m.wizard {
		return nil
	}
	return func() tea.Msg { return startMsg{} }
}

type startMsg struct{}

func (m *model) start() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.id++
	id := m.id
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.events = make(chan event, 128)
	ch := m.events
	c := m.c
	m.s = engine.Snapshot{}
	m.logs = nil
	m.history = nil
	m.offset = 0
	m.wizard = false
	m.err = ""
	go func() {
		defer close(ch)
		_ = engine.Run(ctx, c, func(s engine.Snapshot) {
			select {
			case ch <- event{id, s}:
			case <-ctx.Done():
			}
		})
	}()
	return wait(ch)
}
func wait(ch chan event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil
		}
		return e
	}
}
func (m *model) openWizard(single string) {
	m.original = m.c
	m.s.Done = true
	if m.cancel != nil {
		m.cancel()
	}
	m.id++
	m.wizard = true
	m.step = 0
	m.err = ""
	if single != "" {
		m.fields = []field{fieldFor(single)}
	} else {
		m.fields = []field{fieldFor("protocol"), fieldFor("mode"), fieldFor("target")}
	}
	m.input = m.value(m.fields[0].name)
}
func fieldFor(name string) field {
	switch name {
	case "protocol":
		return field{name, "Network protocol", []string{"icmp", "tcp", "udp"}}
	case "mode":
		return field{name, "probe: latency/reachability · load: throughput to lazyiperf server", []string{"probe", "load"}}
	case "target":
		return field{name, "Target IP or hostname", nil}
	case "port":
		return field{name, "Service port for TCP probes; lazyiperf server port otherwise", nil}
	case "count":
		return field{name, "Maximum probes/datagrams/TCP writes (not TCP wire packets); 0 = duration only", nil}
	case "duration":
		return field{name, "e.g. 10s or 1m; 0s for count-only probes (max 1h)", nil}
	case "interval":
		return field{name, "Time between probes, e.g. 1s or 100ms", nil}
	case "timeout":
		return field{name, "Probe/connect timeout, e.g. 1s (1ms–30s)", nil}
	case "size":
		return field{name, "Payload bytes; 0 = automatic (56 probe, 1200 UDP load, 32768 TCP load)", nil}
	default:
		return field{name, "Load rate in Mbps; 0 = unlimited TCP", nil}
	}
}
func (m model) value(name string) string {
	switch name {
	case "protocol":
		return m.c.Protocol
	case "mode":
		return m.c.Mode
	case "target":
		return m.c.Target
	case "port":
		return strconv.Itoa(m.c.Port)
	case "count":
		return strconv.Itoa(m.c.Count)
	case "duration":
		return m.c.Duration.String()
	case "interval":
		return m.c.Interval.String()
	case "timeout":
		return m.c.Timeout.String()
	case "size":
		return strconv.Itoa(m.c.Size)
	default:
		return strconv.FormatFloat(m.c.Rate, 'f', -1, 64)
	}
}
func (m *model) set(name, v string) error {
	var err error
	switch name {
	case "protocol":
		m.c.Protocol = v
	case "mode":
		m.c.Mode = v
	case "target":
		m.c.Target = strings.TrimSpace(v)
	case "port":
		m.c.Port, err = strconv.Atoi(v)
	case "count":
		m.c.Count, err = strconv.Atoi(v)
	case "duration":
		m.c.Duration, err = time.ParseDuration(v)
	case "interval":
		m.c.Interval, err = time.ParseDuration(v)
	case "timeout":
		m.c.Timeout, err = time.ParseDuration(v)
	case "size":
		m.c.Size, err = strconv.Atoi(v)
	case "rate":
		m.c.Rate, err = strconv.ParseFloat(v, 64)
	}
	return err
}
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case startMsg:
		return m, m.start()
	case event:
		if msg.id != m.id {
			return m, nil
		}
		previous := m.s
		m.s = msg.s
		if msg.s.Log != "" {
			m.logs = append(m.logs, fmt.Sprintf("%6.1fs  %s", msg.s.Elapsed, sanitize(msg.s.Log)))
			if len(m.logs) > 500 {
				m.logs = m.logs[len(m.logs)-500:]
			}
		}
		v := msg.s.RTT
		if m.c.Mode == "load" {
			v = msg.s.Mbps
		}
		if (m.c.Mode == "probe" && msg.s.Received > previous.Received) || (m.c.Mode == "load" && msg.s.Bytes > previous.Bytes) {
			m.history = append(m.history, v)
		}
		if len(m.history) > 240 {
			m.history = m.history[len(m.history)-240:]
		}
		return m, wait(m.events)
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if m.wizard {
			switch key {
			case "esc":
				m.c = m.original
				if m.cancel != nil {
					m.wizard = false
				} else {
					return m, tea.Quit
				}
			case "shift+tab":
				if m.step > 0 {
					m.step--
					if m.fields[m.step].name == "mode" && m.c.Protocol == "icmp" {
						m.step--
					}
					m.input = m.value(m.fields[m.step].name)
					m.err = ""
				}
			case "ctrl+u":
				m.input = ""
			case "backspace":
				r := []rune(m.input)
				if len(r) > 0 {
					m.input = string(r[:len(r)-1])
				}
			case "up", "down", "left", "right", "tab":
				choices := m.fields[m.step].choices
				if len(choices) > 0 {
					idx := 0
					for i, v := range choices {
						if v == m.input {
							idx = i
						}
					}
					delta := 1
					if key == "up" || key == "left" {
						delta = -1
					}
					m.input = choices[(idx+delta+len(choices))%len(choices)]
				}
			case "enter":
				f := m.fields[m.step]
				if err := m.set(f.name, m.input); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.err = ""
				if f.name == "protocol" && m.c.Protocol == "icmp" {
					m.c.Mode = "probe"
					m.step++
				}
				if f.name == "target" && len(m.fields) >= 3 {
					m.fields = m.fields[:3]
					names := []string{}
					if m.c.Protocol != "icmp" {
						names = append(names, "port")
					}
					names = append(names, "duration", "count")
					if m.c.Mode == "probe" {
						names = append(names, "interval")
					}
					names = append(names, "timeout")
					if m.c.Protocol != "tcp" || m.c.Mode == "load" {
						names = append(names, "size")
					}
					if m.c.Mode == "load" {
						names = append(names, "rate")
					}
					for _, n := range names {
						m.fields = append(m.fields, fieldFor(n))
					}
				}
				if m.step+1 < len(m.fields) {
					m.step++
					m.input = m.value(m.fields[m.step].name)
				} else {
					if err := m.c.Validate(); err != nil {
						m.err = err.Error() + " · shift+tab to edit earlier settings"
						return m, nil
					}
					return m, m.start()
				}
			default:
				if msg.Type == tea.KeyRunes {
					for _, r := range msg.Runes {
						if unicode.IsPrint(r) {
							m.input += string(r)
						}
					}
				}
			}
			return m, nil
		}
		switch key {
		case "q":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "r":
			return m, m.start()
		case "s":
			if m.cancel != nil {
				m.cancel()
			}
			m.s.Done = true
			m.logs = append(m.logs, "stopped by user")
		case "e":
			m.openWizard("")
		case "t":
			m.openWizard("target")
		case "c":
			m.openWizard("count")
		case "d":
			m.openWizard("duration")
		case "up", "k":
			m.offset = min(m.offset+1, max(0, len(m.logs)-1))
		case "down", "j":
			m.offset = max(0, m.offset-1)
		case "end":
			m.offset = 0
		}
	}
	return m, nil
}
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func (m model) View() string {
	if m.width < 48 || m.height < 16 {
		return "lazyiperf — resize terminal to at least 48 × 16\nctrl+c quit"
	}
	title := accent.Bold(true).Render(" lazyiperf ") + muted.Render(" NETWORK WORKBENCH")
	if m.wizard {
		f := m.fields[m.step]
		choices := ""
		if len(f.choices) > 0 {
			choices = "\n" + strings.Join(f.choices, " / ") + "  (arrow keys to select)"
		}
		return title + "\n\n" + border.Width(min(80, m.width-6)).Render(fmt.Sprintf("SETUP  %d/%d\n\n%s\n%s%s\n\n> %s▏\n\n%s", m.step+1, len(m.fields), strings.ToUpper(f.name), f.help, choices, sanitize(m.input), m.err)) + "\n\n" + muted.Render(" enter next/start · shift+tab back · ctrl+u clear · esc back/quit · ctrl+c quit")
	}
	s := m.s
	state := "RUNNING"
	if s.Done {
		state = "COMPLETE"
	}
	if s.Error != "" {
		state = "ERROR"
	}
	status := fmt.Sprintf("%s  %s/%s → %s  %.1fs", state, strings.ToUpper(m.c.Protocol), m.c.Mode, sanitize(m.c.Target), s.Elapsed)
	metrics := ""
	graphTitle := "RTT · ms"
	if m.c.Protocol == "tcp" {
		graphTitle = "TCP connect time · ms"
	}
	if m.c.Mode == "probe" {
		metrics = fmt.Sprintf("Sent %d   Replies %d   Failed %d   Loss %.1f%%\nLast %.3f ms   Min %.3f   Mean %.3f   P95 %.3f   Max %.3f\nJitter %.3f ms (smoothed RTT variation)", s.Sent, s.Received, s.Failed, s.Loss, s.RTT, s.Min, s.Mean, s.P95, s.Max, s.Jitter)
	} else {
		graphTitle = "Average transmit throughput · Mbps"
		rx := "awaiting final receiver report"
		if s.ReceiverReported {
			rx = fmt.Sprintf("Receiver %d bytes · %.3f Mbps", s.ReceiverBytes, s.ReceiverMbps)
		}
		metrics = fmt.Sprintf("TX %.3f Mbps   Bytes %d   Writes/datagrams %d   %.0f/s\n%s", s.Mbps, s.Bytes, s.Sent, s.PPS, rx)
		if m.c.Protocol == "udp" && s.ReceiverReported {
			metrics += fmt.Sprintf("\nLoss %.2f%%   Jitter %.3f ms   Duplicate %d   Reordered %d   Too late %d", s.Loss, s.Jitter, s.Duplicates, s.OutOfOrder, s.LateDiscarded)
		} else {
			metrics += "\n"
		}
	}
	w := m.width - 4
	plot := border.Width(w).Render(accent.Render(graphTitle) + "\n" + spark(m.history, max(1, w-4)))
	summary := border.Width(w).Render(accent.Render("METRICS") + "\n" + metrics)
	footer := muted.Render(" r repeat · s stop · e setup · t target · c count · d duration\n ↑/↓ scroll logs · end follow · q quit")
	logHeight := max(1, m.height-lipgloss.Height(title+"\n"+status+"\n"+plot+"\n"+summary+"\n"+footer)-5)
	end := max(0, len(m.logs)-m.offset)
	start := max(0, end-logHeight)
	lines := m.logs[start:end]
	clipped := make([]string, 0, len(lines))
	for _, line := range lines {
		clipped = append(clipped, lipgloss.NewStyle().MaxWidth(w-2).Render(line))
	}
	logs := border.Width(w).Height(logHeight + 1).Render(accent.Render("EVENT LOG") + "\n" + strings.Join(clipped, "\n"))
	return title + "\n" + status + "\n" + plot + "\n" + summary + "\n" + logs + "\n" + footer
}
func spark(values []float64, width int) string {
	if len(values) == 0 {
		return muted.Render("Waiting for samples…")
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	hi := 0.0
	for _, v := range values {
		hi = math.Max(hi, v)
	}
	chars := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for _, v := range values {
		idx := 0
		if hi > 0 {
			idx = int(v / hi * 7)
		}
		b.WriteRune(chars[min(7, max(0, idx))])
	}
	return accent.Render(b.String()) + fmt.Sprintf("\n0 — %.3f  · latest on right", hi)
}
