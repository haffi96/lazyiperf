package engine

import (
	"context"
	"fmt"
	"net"
	"time"
)

type Emit func(Snapshot)

func Run(ctx context.Context, c Config, emit Emit) error {
	if err := c.Validate(); err != nil {
		return err
	}
	m := newMetrics()
	m.LossAvailable = c.Mode == "probe"
	emit(m.snapshot(fmt.Sprintf("%s %s → %s", c.Protocol, c.Mode, c.Address())))
	var err error
	switch {
	case c.Protocol == "icmp":
		err = icmpProbe(ctx, c, m, emit)
	case c.Protocol == "tcp" && c.Mode == "probe":
		err = tcpProbe(ctx, c, m, emit)
	default:
		err = loadOrUDP(ctx, c, m, emit)
	}
	m.Done = true
	if err != nil {
		m.Error = err.Error()
	}
	line := "test complete"
	if err != nil {
		line = err.Error()
	}
	emit(m.snapshot(line))
	return err
}
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func running(c Config, m *metrics) bool {
	return (c.Count == 0 || m.Sent < uint64(c.Count)) && (c.Duration == 0 || time.Since(m.started) < c.Duration)
}
func probeContext(ctx context.Context, c Config) (context.Context, context.CancelFunc) {
	if c.Duration > 0 {
		return context.WithTimeout(ctx, c.Duration)
	}
	return context.WithCancel(ctx)
}
func tcpProbe(ctx context.Context, c Config, m *metrics, emit Emit) error {
	run, cancel := probeContext(ctx, c)
	defer cancel()
	for running(c, m) {
		if run.Err() != nil {
			break
		}
		start := time.Now()
		m.Sent++
		conn, err := (&net.Dialer{Timeout: c.Timeout}).DialContext(run, "tcp", c.Address())
		line := ""
		if err != nil {
			m.Failed++
			line = fmt.Sprintf("seq=%d connection failed: %v", m.Sent, err)
		} else {
			conn.Close()
			m.Received++
			m.latency(time.Since(start))
			line = fmt.Sprintf("seq=%d connected time=%.3f ms", m.Sent, m.RTT)
		}
		emit(m.snapshot(line))
		if !running(c, m) {
			break
		}
		if sleep(run, c.Interval-time.Since(start)) != nil {
			break
		}
	}
	return ctx.Err()
}
