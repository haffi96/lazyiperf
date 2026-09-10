package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

func icmpProbe(ctx context.Context, c Config, m *metrics, emit Emit) error {
	run, cancel := probeContext(ctx, c)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(run, c.Target)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("no addresses for %s", c.Target)
	}
	ip := ips[0]
	network, bind, proto := "udp4", "0.0.0.0", 1
	var request icmp.Type = ipv4.ICMPTypeEcho
	var reply icmp.Type = ipv4.ICMPTypeEchoReply
	if ip.IP.To4() == nil {
		network, bind, proto = "udp6", "::", 58
		request = ipv6.ICMPTypeEchoRequest
		reply = ipv6.ICMPTypeEchoReply
	}
	pc, err := icmp.ListenPacket(network, bind)
	if err != nil {
		return fmt.Errorf("open ICMP socket: %w (Linux may require net.ipv4.ping_group_range to include your group)", err)
	}
	defer pc.Close()
	stop := context.AfterFunc(run, func() { pc.Close() })
	defer stop()
	data := make([]byte, c.PayloadSize())
	if _, err = rand.Read(data); err != nil {
		return err
	}
	buf := make([]byte, 65535)
	for running(c, m) && run.Err() == nil {
		start := time.Now()
		m.Sent++
		seq := int(m.Sent % 65536)
		// Unique payload validates replies even when the OS rewrites the echo identifier.
		nonce := make([]byte, 16)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		copy(data, nonce)
		wire, err := (&icmp.Message{Type: request, Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: seq, Data: data}}).Marshal(nil)
		if err != nil {
			return err
		}
		deadline := start.Add(c.Timeout)
		if d, ok := run.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		pc.SetDeadline(deadline)
		_, err = pc.WriteTo(wire, &net.UDPAddr{IP: ip.IP, Zone: ip.Zone})
		success := false
		if err == nil {
			for {
				n, peer, e := pc.ReadFrom(buf)
				if e != nil {
					err = e
					break
				}
				var peerIP net.IP
				switch p := peer.(type) {
				case *net.UDPAddr:
					peerIP = p.IP
				case *net.IPAddr:
					peerIP = p.IP
				}
				if !peerIP.Equal(ip.IP) {
					continue
				}
				msg, e := icmp.ParseMessage(proto, buf[:n])
				if e != nil || msg.Type != reply {
					continue
				}
				echo, ok := msg.Body.(*icmp.Echo)
				if !ok || echo.Seq != seq || !bytes.Equal(echo.Data, data) {
					continue
				}
				success = true
				break
			}
		}
		line := ""
		if success {
			m.Received++
			m.Bytes += uint64(len(data))
			m.latency(time.Since(start))
			line = fmt.Sprintf("%d bytes from %s: seq=%d time=%.3f ms", len(data), ip.IP, m.Sent, m.RTT)
		} else {
			m.Failed++
			line = fmt.Sprintf("seq=%d timeout/error: %v", m.Sent, err)
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
