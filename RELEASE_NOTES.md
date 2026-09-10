Initial native Go release for macOS and Linux (amd64 and arm64).

- Interactive setup and a keyboard-driven TUI with metrics, history graph, and event log.
- Native ICMP echo and TCP connection probes.
- Native UDP echo and TCP/UDP upload tests using `lazyiperf server`.
- Repeat and edit tests; equivalent CLI flags, plain text, and JSON output.
- OS/architecture detection and SHA-256 verification in the curl installer.

Install:

```sh
curl -fsSL https://raw.githubusercontent.com/haffi96/lazyiperf/main/scripts/install.sh | sh
```

This first version does not support existing iperf3 servers, reverse tests, parallel streams, or raw hping3 packet crafting. UDP load receiver metrics arrive at test completion. Read the README for metric definitions and server networking requirements.
