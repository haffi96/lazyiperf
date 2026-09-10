package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/haffi96/lazyiperf/internal/engine"
)

func TestWizardAndStaleEvents(t *testing.T) {
	m := model{ctx: context.Background(), c: engine.Default(), width: 100, height: 30}
	m.openWizard("")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.fields[m.step].name != "target" {
		t.Fatal("ICMP should skip mode")
	}
	m.input = "127.0.0.1"
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.fields[m.step].name != "duration" {
		t.Fatal("ICMP should skip port")
	}
	next, _ = m.Update(event{id: m.id - 1, s: engine.Snapshot{Sent: 99}})
	if next.(model).s.Sent != 0 {
		t.Fatal("stale run overwrote current state")
	}
	if !strings.Contains(m.View(), "SETUP") {
		t.Fatal("missing wizard")
	}
}
func TestEditEscapeRestoresConfig(t *testing.T) {
	m := model{ctx: context.Background(), c: engine.Default(), cancel: func() {}}
	m.c.Target = "first"
	m.openWizard("target")
	m.c.Target = "second"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(model).c.Target != "first" {
		t.Fatal("escape did not restore config")
	}
}
