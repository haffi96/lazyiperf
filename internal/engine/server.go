package engine

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"
)

// Serve accepts at most 32 active sessions. Cancelling ctx closes all sessions.
func Serve(ctx context.Context, ln net.Listener, log func(string)) error {
	stop := context.AfterFunc(ctx, func() { ln.Close() })
	defer stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, 32)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			defer conn.Close()
			stop := context.AfterFunc(ctx, func() { conn.Close() })
			defer stop()
			if err := serveSession(conn); err != nil && ctx.Err() == nil && log != nil {
				log(fmt.Sprintf("%s: %v", conn.RemoteAddr(), err))
			}
		}()
	}
}
func serveSession(conn net.Conn) error {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReaderSize(conn, 8192)
	var h hello
	if err := readJSON(r, &h); err != nil {
		return err
	}
	reject := func(err error) error { writeJSON(conn, ready{Error: err.Error()}); return err }
	if h.Version != protocolVersion {
		return reject(fmt.Errorf("unsupported protocol; use a matching lazyiperf client"))
	}
	c := h.Config
	if err := c.Validate(); err != nil {
		return reject(err)
	}
	if c.Protocol == "icmp" || (c.Protocol == "tcp" && c.Mode != "load") {
		return reject(fmt.Errorf("server expects TCP load or UDP probe/load"))
	}
	duration := c.Duration
	if duration == 0 {
		duration = time.Hour
	}
	conn.SetDeadline(time.Now().Add(duration + c.Timeout + 5*time.Second))
	if c.Protocol == "tcp" {
		if err := writeJSON(conn, ready{Version: protocolVersion}); err != nil {
			return err
		}
		n, err := io.Copy(io.Discard, r)
		if err != nil {
			return err
		}
		return writeJSON(conn, report{Bytes: uint64(n)})
	}
	local := conn.LocalAddr().(*net.TCPAddr)
	network := "udp4"
	if local.IP.To4() == nil {
		network = "udp6"
	}
	pc, err := net.ListenUDP(network, &net.UDPAddr{IP: local.IP, Zone: local.Zone})
	if err != nil {
		return reject(err)
	}
	defer pc.Close()
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return err
	}
	if err = writeJSON(conn, ready{Version: protocolVersion, UDPPort: pc.LocalAddr().(*net.UDPAddr).Port, Token: token}); err != nil {
		return err
	}
	pc.SetReadDeadline(time.Now().Add(duration + c.Timeout + 5*time.Second))
	results := make(chan report, 1)
	go func() {
		var result report
		var window sequenceWindow
		var lastTransit float64
		haveTransit := false
		buf := make([]byte, 65535)
		peerIP := conn.RemoteAddr().(*net.TCPAddr).IP
		for {
			n, peer, e := pc.ReadFromUDP(buf)
			if e != nil {
				break
			}
			if n < 32 || !peer.IP.Equal(peerIP) || !bytes.Equal(buf[:16], token) {
				continue
			}
			seq := binary.BigEndian.Uint64(buf[16:24])
			dup, ooo, old := window.add(seq)
			if dup {
				result.Duplicates++
				continue
			}
			if old {
				result.LateDiscarded++
				continue
			}
			if ooo {
				result.OutOfOrder++
			}
			result.Packets++
			result.Bytes += uint64(n)
			transit := float64(time.Now().UnixNano()-int64(binary.BigEndian.Uint64(buf[24:32]))) / 1e6
			if haveTransit {
				result.Jitter += (math.Abs(transit-lastTransit) - result.Jitter) / 16
			}
			lastTransit = transit
			haveTransit = true
			if c.Mode == "probe" {
				pc.SetWriteDeadline(time.Now().Add(c.Timeout))
				pc.WriteToUDP(buf[:n], peer)
			}
		}
		results <- result
	}()
	// EOF on the control connection marks the end of the sender stream.
	_, readErr := io.Copy(io.Discard, io.LimitReader(r, 1))
	pc.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	result := <-results
	if readErr != nil {
		return readErr
	}
	return writeJSON(conn, result)
}
