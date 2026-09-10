package engine

import (
	"math"
	"sort"
	"time"
)

type Snapshot struct {
	Elapsed          float64 `json:"elapsed_seconds"`
	Sent             uint64  `json:"sent"`
	Received         uint64  `json:"received"`
	Failed           uint64  `json:"failed"`
	Bytes            uint64  `json:"bytes_sent"`
	ReceiverBytes    uint64  `json:"receiver_bytes"`
	ReceiverReported bool    `json:"receiver_reported"`
	Duplicates       uint64  `json:"duplicates"`
	OutOfOrder       uint64  `json:"out_of_order"`
	LateDiscarded    uint64  `json:"late_discarded"`
	LossAvailable    bool    `json:"loss_available"`
	RTTAvailable     bool    `json:"rtt_available"`
	Loss             float64 `json:"loss_percent"`
	RTT              float64 `json:"rtt_ms"`
	Min              float64 `json:"min_ms"`
	Mean             float64 `json:"mean_ms"`
	P95              float64 `json:"p95_ms"`
	Max              float64 `json:"max_ms"`
	Jitter           float64 `json:"jitter_ms"`
	Mbps             float64 `json:"send_mbps"`
	ReceiverMbps     float64 `json:"receiver_mbps"`
	PPS              float64 `json:"sent_per_second"`
	Log              string  `json:"log,omitempty"`
	Done             bool    `json:"done"`
	Error            string  `json:"error,omitempty"`
}
type metrics struct {
	Snapshot
	started       time.Time
	ended         time.Time
	samples       []float64
	sum, previous float64
	rttCount      uint64
}

func newMetrics() *metrics { return &metrics{started: time.Now()} }
func (m *metrics) latency(d time.Duration) {
	v := float64(d) / float64(time.Millisecond)
	m.RTTAvailable = true
	m.RTT = v
	m.rttCount++
	m.sum += v
	if m.rttCount == 1 {
		m.Min = v
	} else {
		m.Jitter += (math.Abs(v-m.previous) - m.Jitter) / 16
	}
	m.previous = v
	m.Min = math.Min(m.Min, v)
	m.Max = math.Max(m.Max, v)
	m.Mean = m.sum / float64(m.rttCount)
	if len(m.samples) == 4096 {
		copy(m.samples, m.samples[1:])
		m.samples = m.samples[:4095]
	}
	m.samples = append(m.samples, v)
}
func (m *metrics) snapshot(log string) Snapshot {
	s := m.Snapshot
	s.Elapsed = time.Since(m.started).Seconds()
	if !m.ended.IsZero() {
		s.Elapsed = m.ended.Sub(m.started).Seconds()
	}
	s.Log = log
	if s.Elapsed > 0 {
		s.Mbps = float64(s.Bytes) * 8 / s.Elapsed / 1e6
		s.ReceiverMbps = float64(s.ReceiverBytes) * 8 / s.Elapsed / 1e6
		s.PPS = float64(s.Sent) / s.Elapsed
	}
	if s.LossAvailable && s.Sent > 0 && s.Received <= s.Sent {
		s.Loss = 100 * float64(s.Sent-s.Received) / float64(s.Sent)
	}
	if len(m.samples) > 0 {
		v := append([]float64(nil), m.samples...)
		sort.Float64s(v)
		s.P95 = v[int(math.Ceil(.95*float64(len(v))))-1]
	}
	return s
}
