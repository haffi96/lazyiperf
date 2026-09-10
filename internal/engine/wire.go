package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

const protocolVersion = "lazyiperf/1"

type hello struct {
	Version string `json:"version"`
	Config  Config `json:"config"`
}
type ready struct {
	Version string `json:"version"`
	UDPPort int    `json:"udp_port,omitempty"`
	Token   []byte `json:"token,omitempty"`
	Error   string `json:"error,omitempty"`
}
type report struct {
	Bytes         uint64  `json:"bytes"`
	Packets       uint64  `json:"packets"`
	Duplicates    uint64  `json:"duplicates"`
	OutOfOrder    uint64  `json:"out_of_order"`
	LateDiscarded uint64  `json:"late_discarded"`
	Jitter        float64 `json:"jitter_ms"`
}

func readJSON(r *bufio.Reader, v any) error {
	b, err := r.ReadSlice('\n')
	if err != nil {
		return fmt.Errorf("read lazyiperf protocol: %w", err)
	}
	if err = json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("invalid lazyiperf message: %w", err)
	}
	return nil
}
func writeJSON(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }

// A fixed sliding window bounds memory even for long, high-rate UDP sessions.
type sequenceWindow struct {
	slots [65536]uint64
	high  uint64
}

func (w *sequenceWindow) add(seq uint64) (duplicate, outOfOrder, tooOld bool) {
	if seq == 0 {
		return false, false, true
	}
	if w.high >= 65536 && seq <= w.high-65536 {
		return false, false, true
	}
	i := seq % 65536
	if w.slots[i] == seq {
		return true, false, false
	}
	w.slots[i] = seq
	outOfOrder = seq < w.high
	if seq > w.high {
		w.high = seq
	}
	return false, outOfOrder, false
}
