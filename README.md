# lazyiperf

A standalone network testing CLI for macOS and Linux, inspired by [lazygit](https://github.com/jesseduffield/lazygit) and [lazydocker](https://github.com/jesseduffield/lazydocker).

Run `lazyiperf` to choose a protocol, test mode, target, and settings. The terminal dashboard shows network metrics, a history graph, and a scrolling event log. Repeat a test or change its settings without leaving the dashboard. Every setup option is also available as a CLI flag.

Written in Go, with native networking and a Bubble Tea TUI. No installed `ping`, `iperf3`, or `hping3` is required.

## Install

Supports **macOS and Linux**, on **amd64 and arm64** (including Apple Silicon).

```sh
curl -fsSL https://raw.githubusercontent.com/haffi96/lazyiperf/main/scripts/install.sh | sh
```

The installer downloads the latest [GitHub release](https://github.com/haffi96/lazyiperf/releases/latest), verifies its SHA-256 checksum, and installs to `~/.local/bin` without sudo. No Go installation is needed. Run the same command to update.

If `lazyiperf` is not found after installation, add the install directory to your shell's `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Add that line to your shell configuration to keep it across sessions. To choose another writable install directory:

```sh
curl -fsSL https://raw.githubusercontent.com/haffi96/lazyiperf/main/scripts/install.sh | LAZYIPERF_BIN_DIR="$HOME/bin" sh
```

## Quick start

Open the interactive setup:

```sh
lazyiperf
```

Choose ICMP, TCP, or UDP, then answer the relevant questions. ICMP supports probes; TCP and UDP support probes and load tests. Supplying `--target` skips the wizard and opens the dashboard with your chosen flags and defaults.

### Probe a target

ICMP echo needs no companion server:

```sh
lazyiperf --target 127.0.0.1 --count 5 --duration 0s
```

TCP probes measure connection establishment time against a listening service. For example, if a local service is listening on port 443:

```sh
lazyiperf --protocol tcp --target 127.0.0.1 --port 443 --count 10 --duration 0s
```

Replace the loopback address with the IP or hostname you want to test. IPv4 and IPv6 are supported.

### Run a load test

TCP/UDP load tests and UDP echo probes require **`lazyiperf server` on the target**. Start it in a separate terminal:

```sh
lazyiperf server --listen :5202
```

Then run a client. These examples use a server on the same machine; replace `127.0.0.1` with the server's address to test between machines.

```sh
# TCP upload at 10 Mbps for 10 seconds
lazyiperf --protocol tcp --mode load --target 127.0.0.1 --duration 10s --rate 10

# TCP upload without a configured rate limit
lazyiperf --protocol tcp --mode load --target 127.0.0.1 --duration 10s --rate 0

# UDP upload, with receiver loss, jitter, and reordering in the final report
lazyiperf --protocol udp --mode load --target 127.0.0.1 --duration 10s --rate 10

# UDP echo probes
lazyiperf --protocol udp --target 127.0.0.1 --count 5 --duration 0s
```

Existing iperf3 servers are not supported. The native client and server use the lazyiperf protocol.

### Use plain text or JSON

Add `--plain` for classic terminal logs, or `--json` for newline-delimited JSON. Both bypass the TUI and require `--target`.

```sh
lazyiperf --target 127.0.0.1 --count 3 --duration 0s --plain
lazyiperf --target 127.0.0.1 --count 3 --duration 0s --json
```

For these output modes, configuration, setup, protocol failures, and cancellation return a nonzero exit status. Failed probes are recorded in the results; even an all-timeout probe run can exit successfully. Use the result fields to decide whether a target met your requirements. In the TUI, test errors appear in the dashboard.

## Options

Use `lazyiperf --help` or `lazyiperf server --help` for the full command help.

- `--protocol`: `icmp` (default), `tcp`, or `udp`.
- `--mode`: `probe` (default) or `load`; ICMP supports probes only.
- `--target`: target IP or hostname.
- `--port`: TCP service or native server port; default `5202`. Not used for ICMP.
- `--duration`: default `10s`, maximum `1h`. Use `0s` with a positive count for count-only probes. Load tests require a positive duration.
- `--count`: maximum probes, UDP datagrams, or TCP application writes; default `0` uses duration only. TCP writes are not wire packet counts.
- `--interval`: time between probes; default `1s`.
- `--timeout`: probe/connect timeout; default `1s`.
- `--size`: payload bytes; default `0` selects 56 for probes, 1200 for UDP load, or 32768 for TCP load. Not used by TCP connection probes.
- `--rate`: load rate in Mbps; default `10`. Only TCP allows `0` for unlimited sending.

When both count and duration are set, the first limit reached ends sending. Use `--duration 0s` when a probe should stop solely on count. Timeouts and final report collection can extend total wall-clock runtime beyond the sending duration.

## Dashboard controls

- `r`: repeat; `s`: stop; `q` or Ctrl+C: quit.
- `e`: full setup; `t`: change target; `c`: change count; `d`: change duration.
- Up/down or `k`/`j`: scroll logs; End: follow newest output.
- In setup: arrows select choices, Enter advances, Shift+Tab goes back, Ctrl+U clears, Esc cancels setup or exits the initial wizard.

Opening setup stops the active test. Completing setup starts a fresh run; cancelling an edit restores the previous configuration without resuming the stopped test. The log retains 500 lines and the graph retains 240 samples. The minimum terminal size is 48 columns by 16 rows.

## Metrics

**Probe tests** show sent/reply/failure counts, loss percentage, latest/minimum/mean/p95/maximum latency, and jitter. ICMP measures echo RTT, TCP measures connection establishment time, and UDP measures application echo RTT. Probe loss means unsuccessful attempts: a refused TCP connection is a failed probe, not proof of a dropped IP packet.

**Load tests** show average transmit throughput, sent bytes, and writes or datagrams per second. Receiver byte counts arrive at completion. UDP also reports unique received datagrams, loss, duplicates, reordering, and receiver arrival jitter. The throughput graph shows the running average. RTT is not measured during load tests.

<details>
<summary>Measurement definitions and limits</summary>

- Probe min/mean/max cover the whole run. P95 uses the most recent 4096 successful samples. Probe jitter is smoothed successive RTT variation: `J += (abs(RTT - previous RTT) - J) / 16`.
- Load throughput divides application bytes by sender elapsed time, excluding connection setup and final report collection. Receiver throughput uses the same sender time window. These are not link-layer bandwidth measurements.
- UDP byte counts include the 32-byte native session/sequence/timestamp header and exclude IP/UDP headers. TCP counts payload bytes and application writes.
- UDP receiver jitter smooths changes in transit time. Constant clock offset cancels out; clock adjustments during a test can affect the result.
- The UDP receiver tracks a 65536-sequence window. Older datagrams are excluded from unique reception counts and reported as `late_discarded`. The server allows 200 ms for late arrival after the sender finishes.
- JSON includes `loss_available`, `rtt_available`, and `receiver_reported`. Check these before interpreting unavailable or zero-valued metrics. UDP load loss is available only after the receiver report arrives.
- Native TCP does not currently expose retransmissions, congestion window, or packet loss.

</details>

## Server networking and ICMP permissions

The server listens on TCP port `5202` by default. TCP load uses that connection; UDP sessions negotiate an ephemeral UDP port over it. Firewalls must allow the TCP control port and negotiated UDP ports between the hosts.

The server supports up to 32 simultaneous sessions. UDP sessions use random tokens and accept packets only from the control connection's peer IP. There is no user authentication or encryption; use it on trusted networks. `--listen :5202` binds to all available interfaces; use `--listen IP:5202` to choose an interface or `--listen 127.0.0.1:5202` for local tests.

ICMP uses native unprivileged datagram sockets. On Linux, a `socket: permission denied` error may mean `net.ipv4.ping_group_range` does not include your user's group. An administrator can configure that setting. The application and installer do not change system permissions.

## Development

Requires **Go 1.26 or newer**, as specified in [go.mod](go.mod).

```sh
git clone https://github.com/haffi96/lazyiperf.git
cd lazyiperf
make build
./bin/lazyiperf
```

Validate changes with:

```sh
go test -race ./...
go vet ./...
```

Tests exercise native TCP/UDP sessions on loopback, byte accounting, cancellation, protocol validation, metrics, and wizard behavior. GitHub Actions runs tests, vet, builds, and an ICMP loopback smoke check on both Linux and macOS. The Linux CI runner explicitly configures its ICMP socket permission before that check.

The CLI entry point is in `cmd/lazyiperf`, networking and metrics in `internal/engine`, and the Bubble Tea dashboard in `internal/ui`.

## Releases

Update [RELEASE_NOTES.md](RELEASE_NOTES.md), then push a new `v*` tag. After Linux and macOS checks pass, GitHub Actions builds and publishes all four OS/architecture binaries, `install.sh`, and `checksums.txt` to GitHub Releases.

To build release assets locally without publishing:

```sh
./scripts/release.sh dev
```

Assets are written to `dist/`. The installer defaults to `haffi96/lazyiperf`; `LAZYIPERF_REPO=owner/repo` selects another repository, and `LAZYIPERF_RELEASE_URL=https://host/release-directory` overrides the download location.

## Not yet implemented

Native iperf3-server compatibility, reverse/download tests, parallel streams, live receiver reports during load tests, saved presets, and raw hping3-style packet crafting are possible next steps. They are not part of the current release.
