package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

func loadOrUDP(ctx context.Context, c Config, m *metrics, emit Emit) error {
	conn, err := (&net.Dialer{Timeout: c.Timeout}).DialContext(ctx, "tcp", c.Address())
	if err != nil {
		return fmt.Errorf("connect to lazyiperf server: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	conn.SetDeadline(time.Now().Add(c.Timeout))
	if err = writeJSON(conn, hello{Version: protocolVersion, Config: c}); err != nil {
		return err
	}
	r := bufio.NewReaderSize(conn, 8192)
	var start ready
	if err = readJSON(r, &start); err != nil {
		return fmt.Errorf("handshake failed (this mode needs lazyiperf server, not iperf3): %w", err)
	}
	if start.Error != "" {
		return fmt.Errorf("server: %s", start.Error)
	}
	if start.Version != protocolVersion {
		return fmt.Errorf("unsupported server version")
	}
	duration := c.Duration
	if duration == 0 {
		duration = time.Hour
	}
	conn.SetDeadline(time.Now().Add(duration + c.Timeout + 3*time.Second))
	var udp *net.UDPConn
	if c.Protocol == "udp" {
		if start.UDPPort < 1 || start.UDPPort > 65535 || len(start.Token) != 16 {
			return fmt.Errorf("invalid UDP handshake")
		}
		peer := conn.RemoteAddr().(*net.TCPAddr)
		udp, err = net.DialUDP("udp", nil, &net.UDPAddr{IP: peer.IP, Zone: peer.Zone, Port: start.UDPPort})
		if err != nil {
			return err
		}
		defer udp.Close()
		stopUDP := context.AfterFunc(ctx, func() { udp.Close() })
		defer stopUDP()
	}
	m.started = time.Now()
	lastEmit := m.started
	payload := make([]byte, c.PayloadSize())
	if udp != nil {
		copy(payload, start.Token)
	}
	deadline := m.started.Add(duration)
	for running(c, m) {
		if err = ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			break
		}
		packetStart := time.Now()
		next := m.Sent + 1
		if udp != nil {
			binary.BigEndian.PutUint64(payload[16:24], next)
			binary.BigEndian.PutUint64(payload[24:32], uint64(time.Now().UnixNano()))
		}
		writeDeadline := time.Now().Add(c.Timeout)
		if deadline.Before(writeDeadline) {
			writeDeadline = deadline
		}
		var n int
		if udp != nil {
			udp.SetWriteDeadline(writeDeadline)
			n, err = udp.Write(payload)
		} else {
			conn.SetWriteDeadline(writeDeadline)
			n, err = conn.Write(payload)
		}
		m.Bytes += uint64(n)
		if err != nil {
			if time.Now().After(deadline) && ctx.Err() == nil {
				break
			}
			return err
		}
		m.Sent++
		if c.Mode == "probe" {
			udp.SetReadDeadline(writeDeadline)
			buf := make([]byte, 65535)
			ok := false
			for {
				n, e := udp.Read(buf)
				if e != nil {
					break
				}
				if n == len(payload) && bytes.Equal(buf[:n], payload) {
					ok = true
					break
				}
			}
			if ok {
				m.Received++
				m.latency(time.Since(packetStart))
			} else {
				m.Failed++
			}
			line := fmt.Sprintf("seq=%d timeout", m.Sent)
			if ok {
				line = fmt.Sprintf("seq=%d UDP echo time=%.3f ms", m.Sent, m.RTT)
			}
			emit(m.snapshot(line))
			if !running(c, m) {
				break
			}
			wait := c.Interval - time.Since(packetStart)
			if remaining := time.Until(deadline); wait > remaining {
				wait = remaining
			}
			if err = sleep(ctx, wait); err != nil {
				return err
			}
		} else {
			if time.Since(lastEmit) >= time.Second {
				emit(m.snapshot(fmt.Sprintf("sent %d bytes in %d writes/datagrams", m.Bytes, m.Sent)))
				lastEmit = time.Now()
			}
			if c.Rate > 0 {
				due := m.started.Add(time.Duration(float64(m.Bytes) * 8 / (c.Rate * 1e6) * float64(time.Second)))
				if due.After(deadline) {
					due = deadline
				}
				if err = sleep(ctx, time.Until(due)); err != nil {
					return err
				}
			}
		}
	}
	m.ended = time.Now()
	if err = conn.(*net.TCPConn).CloseWrite(); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(c.Timeout + time.Second))
	var result report
	if err = readJSON(r, &result); err != nil {
		return fmt.Errorf("receiver report unavailable: %w", err)
	}
	m.ReceiverReported = true
	m.ReceiverBytes = result.Bytes
	m.Duplicates = result.Duplicates
	m.OutOfOrder = result.OutOfOrder
	m.LateDiscarded = result.LateDiscarded
	if c.Protocol == "udp" && c.Mode == "load" {
		m.LossAvailable = true
		m.Received = result.Packets
		m.Jitter = result.Jitter
	}
	return ctx.Err()
}
