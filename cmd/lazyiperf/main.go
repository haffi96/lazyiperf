package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/haffi96/lazyiperf/internal/engine"
	"github.com/haffi96/lazyiperf/internal/ui"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazyiperf:", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if len(os.Args) > 1 && os.Args[1] == "server" {
		fs := flag.NewFlagSet("lazyiperf server", flag.ContinueOnError)
		listen := fs.String("listen", ":5202", "TCP control/data address; UDP sessions use ephemeral ports")
		if err := fs.Parse(os.Args[2:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		if fs.NArg() > 0 {
			return fmt.Errorf("unexpected server arguments")
		}
		ln, err := net.Listen("tcp", *listen)
		if err != nil {
			return err
		}
		defer ln.Close()
		fmt.Printf("lazyiperf %s server listening on %s\n", version, ln.Addr())
		return engine.Serve(ctx, ln, func(s string) { fmt.Fprintln(os.Stderr, s) })
	}
	c := engine.Default()
	fs := flag.NewFlagSet("lazyiperf", flag.ContinueOnError)
	fs.StringVar(&c.Protocol, "protocol", c.Protocol, "icmp, tcp, or udp")
	fs.StringVar(&c.Mode, "mode", c.Mode, "probe or load (ICMP: probe only)")
	fs.StringVar(&c.Target, "target", "", "target IP or hostname")
	fs.IntVar(&c.Port, "port", c.Port, "target TCP port or lazyiperf server port")
	fs.IntVar(&c.Count, "count", c.Count, "stop after N probes/datagrams/TCP writes; 0 uses duration")
	fs.DurationVar(&c.Duration, "duration", c.Duration, "test duration, up to 1h; 0 for count-only probes")
	fs.DurationVar(&c.Interval, "interval", c.Interval, "interval between probes")
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, "probe/connect timeout")
	fs.IntVar(&c.Size, "size", c.Size, "payload bytes; 0 chooses mode default")
	fs.Float64Var(&c.Rate, "rate", c.Rate, "load rate in Mbps; TCP allows 0 for unlimited")
	plain := fs.Bool("plain", false, "print classic text output without a TUI")
	jsonOut := fs.Bool("json", false, "stream newline-delimited JSON without a TUI")
	showVersion := fs.Bool("version", false, "print version")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "lazyiperf — interactive network testing\n\nUsage: lazyiperf [flags]\n       lazyiperf server [--listen :5202]\n\nNo target opens the wizard. A target starts the dashboard.\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if *plain && *jsonOut {
		return fmt.Errorf("choose --plain or --json")
	}
	if *plain || *jsonOut {
		if err := c.Validate(); err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		var outputErr error
		err := engine.Run(ctx, c, func(s engine.Snapshot) {
			if outputErr != nil {
				return
			}
			if *jsonOut {
				outputErr = enc.Encode(s)
			} else {
				fmt.Printf("[%6.2fs] %s\n", s.Elapsed, s.Log)
				if s.Done {
					if c.Mode == "probe" {
						fmt.Printf("sent=%d received=%d loss=%.1f%% RTT min/avg/max=%.3f/%.3f/%.3f ms jitter=%.3f ms\n", s.Sent, s.Received, s.Loss, s.Min, s.Mean, s.Max, s.Jitter)
					} else {
						fmt.Printf("sent=%d bytes avg=%.3f Mbps receiver=%d bytes report=%t\n", s.Bytes, s.Mbps, s.ReceiverBytes, s.ReceiverReported)
						if c.Protocol == "udp" && s.ReceiverReported {
							fmt.Printf("datagrams sent=%d received=%d loss=%.2f%% jitter=%.3f ms\n", s.Sent, s.Received, s.Loss, s.Jitter)
						}
					}
				}
			}
		})
		if outputErr != nil {
			return outputErr
		}
		return err
	}
	return ui.Run(ctx, c)
}
