# lazyiperf

A native Go network testing CLI for macOS and Linux, inspired by lazygit and lazydocker. Run `lazyiperf` for guided setup, then watch a live terminal dashboard with metrics, a history graph, and a scrolling event log.

This is an initial working implementation. It does not invoke ping, hping3, or iperf3, and does not yet speak the iperf3 wire protocol.

## Run from source

```sh
go build -o bin/lazyiperf ./cmd/lazyiperf
./bin/lazyiperf
```

See `go.mod` for the required Go version. Release binaries need no Go installation, C runtime dependency, or external network tools.

## Tests and flags

Every wizard setting has a flag. Supplying `--target` skips setup and opens the dashboard. Add `--plain` for classic terminal output, or `--json` for newline-delimited JSON suitable for scripts. Go's flag parser accepts both `--flag value` and `--flag=value`.

```sh
# ICMP echo, no companion server needed
lazyiperf --target 127.0.0.1 --count 5 --duration 0s

# TCP connection latency against a listening service
lazyiperf --protocol tcp --target 127.0.0.1 --port 443 --count 10 --duration 0s

# Start a native server on a machine you want to test
lazyiperf server --listen :5202

# TCP upload, 10 Mbps for 10 seconds (use --rate 0 for unlimited TCP)
lazyiperf --protocol tcp --mode load --target 127.0.0.1 --duration 10s --rate 10

# UDP upload with receiver-side loss, jitter, and reordering statistics
lazyiperf --protocol udp --mode load --target 127.0.0.1 --duration 10s --rate 10

# UDP echo through the native server
lazyiperf --protocol udp --target 127.0.0.1 --count 5 --duration 0s --plain

# Machine-readable results
lazyiperf --target 127.0.0.1 --count 3 --duration 0s --json
```

Common options: `--protocol`, `--mode`, `--target`, `--port`, `--count`, `--duration`, `--interval`, `--timeout`, `--size`, `--rate`. Use `--help` for defaults and bounds. If both count and duration are set, the first limit reached ends sending. Load tests always require a duration, up to one hour. The default is 10 seconds. Count means probes, UDP datagrams, or TCP application writes—not TCP packets on the wire.

## Dashboard

- `r`: repeat; `s`: stop; `q` or Ctrl+C: quit.
- `e`: full setup; `t`: change target; `c`: change count; `d`: change duration.
- Up/down or `k`/`j`: scroll logs; End: follow newest output.
- In setup: arrows select choices, Enter advances, Shift+Tab goes back, Ctrl+U clears, Esc cancels.

Changing settings stops the active test; finishing setup starts a fresh run. The event log retains 500 lines and the graph retains 240 samples. Minimum terminal size is 48 columns by 16 rows.

## What the measurements mean

ICMP reports echo RTT; TCP probes report connection establishment time; UDP probes report application echo RTT. These are different measurements. Probe loss is the percentage of unsuccessful attempts, so a refused TCP connection counts as a failed probe, not evidence of a dropped IP packet.

Probe metrics include sent/reply/failure counts, min/mean/p95/max latency and smoothed successive RTT variation (`J += (abs(RTT - previous RTT) - J) / 16`). Min/mean/max cover the whole run; p95 uses the most recent 4096 successful samples.

Load throughput is application bytes divided by sender elapsed time, excluding connection setup and final report collection. The current graph shows this running average. It is not link-layer bandwidth. UDP byte counts include the 32-byte native session/sequence/timestamp header, but exclude IP/UDP headers. TCP counts payload bytes and application writes. Native TCP does not currently expose retransmissions, congestion window, or packet loss.

The server reports actual received bytes after a load test. UDP additionally reports unique datagrams, duplicates, reordering, and receiver arrival jitter using the smoothed change in transit time. Constant clock offset cancels from this jitter calculation; clock adjustments during a test can affect it. Receiver loss is available only when the final report arrives. Datagrams delayed by more than the 65536-sequence tracking window are discarded from unique reception counts and reported separately as `late_discarded`. The server allows 200 ms for late UDP arrival after sending ends.

JSON includes `loss_available`, `rtt_available`, and `receiver_reported`; check these before interpreting zero-valued or unavailable metrics. Probe timeouts are recorded in results; setup/protocol failures and cancellation return a nonzero exit status. An all-timeout probe run still completes normally.

## Server and operating systems

TCP load uses the chosen server port. UDP sessions negotiate an ephemeral UDP port over the TCP control connection; firewalls must allow those UDP ports between the test hosts. Each session has a random token, and UDP packets must come from the control connection's peer IP. This is a local/trusted-network test server, without user authentication or encryption; it supports up to 32 simultaneous sessions. Bind to a specific interface with `--listen IP:5202` when appropriate.

ICMP uses native unprivileged datagram sockets on macOS and Linux. Linux may need `net.ipv4.ping_group_range` configured to include the user's group; the tool reports socket errors and does not change system settings. IPv4 and IPv6 addresses/hostnames are accepted. Raw SYN/ACK crafting, spoofing, and other advanced hping3 features are not implemented.

## Installation and releases

Build binaries for Linux/macOS on amd64/arm64:

```sh
./scripts/release.sh v0.1.0
```

This produces four binaries, `install.sh`, and SHA-256 checksums in `dist/`. Publish those files together to an HTTPS release directory. The installer detects OS/architecture, verifies the downloaded binary checksum, and installs to `~/.local/bin` without sudo. Override with `LAZYIPERF_BIN_DIR`.

Install the latest GitHub release:

```sh
curl -fsSL https://raw.githubusercontent.com/haffi96/lazyiperf/main/scripts/install.sh | sh
```

The default release repository is `haffi96/lazyiperf`. Override with `LAZYIPERF_REPO=owner/repo` or `LAZYIPERF_RELEASE_URL=https://host/release-directory`. Push a `v*` tag to test on macOS/Linux and publish release assets through GitHub Actions.

## Development

```sh
go test -race ./...
go vet ./...
```

The integration tests run TCP/UDP client and server sessions on loopback, check byte accounting, probe results, cancellation, and protocol rejection. They do not generate traffic against external hosts. ICMP is checked manually because CI permissions vary.

Possible next steps: reverse/download tests, parallel streams, interval receiver reports, richer time-axis charts, saved presets and export, OS-specific TCP diagnostics, and optional native iperf3 compatibility after assessing its versioned control/data protocol.
