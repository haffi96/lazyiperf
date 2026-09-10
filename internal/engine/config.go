package engine

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"time"
)

// Config is shared by flags, the wizard, and the native server protocol.
type Config struct {
	Protocol string        `json:"protocol"`
	Mode     string        `json:"mode"`
	Target   string        `json:"target"`
	Port     int           `json:"port"`
	Count    int           `json:"count"`
	Duration time.Duration `json:"duration_ns"`
	Interval time.Duration `json:"interval_ns"`
	Timeout  time.Duration `json:"timeout_ns"`
	Size     int           `json:"size"`
	Rate     float64       `json:"rate_mbps"`
}

func Default() Config {
	return Config{Protocol: "icmp", Mode: "probe", Port: 5202, Duration: 10 * time.Second, Interval: time.Second, Timeout: time.Second, Rate: 10}
}
func (c Config) PayloadSize() int {
	if c.Size > 0 {
		return c.Size
	}
	if c.Mode == "load" {
		if c.Protocol == "tcp" {
			return 32768
		}
		return 1200
	}
	return 56
}
func (c Config) Address() string { return net.JoinHostPort(c.Target, strconv.Itoa(c.Port)) }
func (c Config) Validate() error {
	if c.Protocol != "icmp" && c.Protocol != "tcp" && c.Protocol != "udp" {
		return fmt.Errorf("protocol must be icmp, tcp, or udp")
	}
	if c.Mode != "probe" && c.Mode != "load" {
		return fmt.Errorf("mode must be probe or load")
	}
	if c.Protocol == "icmp" && c.Mode != "probe" {
		return fmt.Errorf("ICMP supports probe mode only")
	}
	if c.Target == "" {
		return fmt.Errorf("target IP or hostname is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be 1–65535")
	}
	if c.Count < 0 {
		return fmt.Errorf("count cannot be negative")
	}
	if c.Duration < 0 || c.Duration > time.Hour {
		return fmt.Errorf("duration must be between 0 and 1h")
	}
	if c.Duration == 0 && c.Count == 0 {
		return fmt.Errorf("set a duration or count")
	}
	if c.Mode == "load" && c.Duration == 0 {
		return fmt.Errorf("load tests require a duration (up to 1h)")
	}
	if c.Timeout < time.Millisecond || c.Timeout > 30*time.Second {
		return fmt.Errorf("timeout must be between 1ms and 30s")
	}
	if c.Interval < time.Millisecond || c.Interval > time.Hour {
		return fmt.Errorf("interval must be between 1ms and 1h")
	}
	if c.Size < 0 || c.PayloadSize() < 32 || c.PayloadSize() > 65507 {
		return fmt.Errorf("size must be 32–65507 bytes, or 0 for automatic")
	}
	if math.IsNaN(c.Rate) || math.IsInf(c.Rate, 0) || c.Rate < 0 || c.Rate > 100000 {
		return fmt.Errorf("rate must be 0–100000 Mbps")
	}
	if c.Protocol == "udp" && c.Mode == "load" && c.Rate == 0 {
		return fmt.Errorf("UDP load requires a positive rate")
	}
	return nil
}
