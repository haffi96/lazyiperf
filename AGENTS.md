# Repository guidance

- Build a standalone native Go CLI for macOS and Linux; do not require external `ping`, `iperf3`, or `hping3` executables.
- Keep CLI parsing in `cmd/lazyiperf`, networking and metrics in `internal/engine`, and Bubble Tea UI code in `internal/ui`.
- Share configuration and validation between flags and the wizard. Keep metric labels, units, and availability accurate.
- Use context cancellation and bounded buffers; stop network activity when a test is stopped or replaced.
- Keep automated network tests on loopback with finite counts or durations.
- Format changed Go files with `gofmt`. For Go changes, run `go test -race ./...` and `go vet ./...`; build with `make build`.
- Update `README.md` when user-facing behavior changes. Release assets are built by `scripts/release.sh`; pushing a `v*` tag publishes through GitHub Actions.
