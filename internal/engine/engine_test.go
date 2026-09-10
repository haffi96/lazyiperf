package engine

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) (string, int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, nil) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(2 * time.Second):
			t.Error("server did not stop")
		}
	})
	a := ln.Addr().(*net.TCPAddr)
	return a.IP.String(), a.Port
}
func TestNativeSessions(t *testing.T) {
	host, port := testServer(t)
	for _, tc := range []struct{ protocol, mode string }{{"tcp", "probe"}, {"tcp", "load"}, {"udp", "probe"}, {"udp", "load"}} {
		t.Run(tc.protocol+"/"+tc.mode, func(t *testing.T) {
			c := Default()
			c.Target = host
			c.Port = port
			c.Protocol = tc.protocol
			c.Mode = tc.mode
			c.Count = 5
			c.Duration = time.Second
			c.Interval = time.Millisecond
			c.Size = 512
			c.Rate = 1
			var s Snapshot
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := Run(ctx, c, func(v Snapshot) { s = v }); err != nil {
				t.Fatal(err)
			}
			if !s.Done || s.Sent != 5 {
				t.Fatalf("unexpected result: %+v", s)
			}
			if c.Mode == "probe" {
				if s.Received != 5 || s.Loss != 0 || !s.RTTAvailable {
					t.Fatalf("probe: %+v", s)
				}
			} else {
				if !s.ReceiverReported || s.Bytes != 2560 || s.ReceiverBytes != s.Bytes {
					t.Fatalf("load: %+v", s)
				}
				if c.Protocol == "tcp" && s.LossAvailable {
					t.Fatal("TCP stream does not measure packet loss")
				}
				if c.Protocol == "udp" && (s.Received != 5 || s.Loss != 0) {
					t.Fatalf("UDP: %+v", s)
				}
			}
		})
	}
}
func TestCancelLoad(t *testing.T) {
	host, port := testServer(t)
	c := Default()
	c.Protocol = "tcp"
	c.Mode = "load"
	c.Target = host
	c.Port = port
	c.Duration = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, c, func(s Snapshot) {
			if s.Sent > 0 {
				cancel()
			}
		})
	}()
	time.AfterFunc(100*time.Millisecond, cancel)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation blocked")
	}
}
func TestSequenceWindow(t *testing.T) {
	var w sequenceWindow
	for _, tc := range []struct {
		seq           uint64
		dup, ooo, old bool
	}{{1, false, false, false}, {3, false, false, false}, {2, false, true, false}, {2, true, false, false}, {70000, false, false, false}, {1, false, false, true}, {70001, false, false, false}} {
		d, o, l := w.add(tc.seq)
		if d != tc.dup || o != tc.ooo || l != tc.old {
			t.Fatalf("seq %d got %v %v %v", tc.seq, d, o, l)
		}
	}
}
func TestMetrics(t *testing.T) {
	m := newMetrics()
	m.LossAvailable = true
	m.Sent = 4
	m.Received = 3
	for _, d := range []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond} {
		m.latency(d)
	}
	s := m.snapshot("")
	if s.Min != 1 || s.Max != 3 || s.Mean != 2 || s.P95 != 3 || s.Loss != 25 {
		t.Fatalf("%+v", s)
	}
}
func TestValidation(t *testing.T) {
	c := Default()
	c.Target = "127.0.0.1"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Protocol = "udp"
	c.Mode = "load"
	c.Rate = 0
	if c.Validate() == nil {
		t.Fatal("unbounded UDP accepted")
	}
	c.Rate = 1
	c.Duration = 0
	if c.Validate() == nil {
		t.Fatal("unbounded duration accepted")
	}
}
func TestRejectForeignProtocol(t *testing.T) {
	host, port := testServer(t)
	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Second))
	writeJSON(conn, hello{Version: "other"})
	var r ready
	if err := readJSON(bufio.NewReader(conn), &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Error, "unsupported") {
		t.Fatalf("%+v", r)
	}
}
