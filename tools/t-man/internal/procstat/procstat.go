// Package procstat resolves per-process resource usage (RSS, CPU%, uptime)
// for a set of PIDs through a single `ps` invocation. It knows nothing about
// launchd or t-man services — callers map labels to PIDs and hand the PIDs
// in, so the same collector can later cover processes t-man does not manage.
package procstat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Stats holds one process's resource usage at collection time.
type Stats struct {
	PID        int
	RSSBytes   int64
	CPUPercent float64
	Uptime     time.Duration
}

// Collector reads process stats via ps.
type Collector struct {
	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewCollector creates a Collector backed by the real ps binary.
func NewCollector() *Collector {
	return &Collector{
		execCommand: exec.CommandContext,
	}
}

// Collect returns stats for the given PIDs from a single ps call, keyed by
// PID. PIDs that have exited are simply absent from the result — ps exiting
// non-zero because none of the requested PIDs exist is not an error.
func (c *Collector) Collect(ctx context.Context, pids []int) (map[int]Stats, error) {
	if len(pids) == 0 {
		return map[int]Stats{}, nil
	}

	pidArgs := make([]string, len(pids))
	for i, pid := range pids {
		pidArgs[i] = strconv.Itoa(pid)
	}

	cmd := c.execCommand(
		ctx,
		"ps",
		"-o",
		"pid=,rss=,pcpu=,etime=",
		"-p",
		strings.Join(pidArgs, ","),
	)
	output, err := cmd.Output()
	if err != nil {
		// ps exits 1 when some (or all) requested PIDs no longer exist;
		// whatever rows it did print are still valid.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, fmt.Errorf("procstat: failed to run ps: %w", err)
		}
	}

	return parsePSOutput(string(output))
}

// parsePSOutput parses `ps -o pid=,rss=,pcpu=,etime=` output: one row per
// process, four whitespace-separated fields, RSS in kilobytes.
func parsePSOutput(output string) (map[int]Stats, error) {
	stats := map[int]Stats{}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("procstat: bad pid %q in ps output: %w", fields[0], err)
		}
		rssKB, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("procstat: bad rss %q in ps output: %w", fields[1], err)
		}
		cpu, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			return nil, fmt.Errorf("procstat: bad pcpu %q in ps output: %w", fields[2], err)
		}
		uptime, err := parseEtime(fields[3])
		if err != nil {
			return nil, err
		}

		stats[pid] = Stats{
			PID:        pid,
			RSSBytes:   rssKB * 1024,
			CPUPercent: cpu,
			Uptime:     uptime,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("procstat: failed to scan ps output: %w", err)
	}

	return stats, nil
}

// parseEtime parses ps elapsed-time values: mm:ss, hh:mm:ss, or dd-hh:mm:ss.
func parseEtime(s string) (time.Duration, error) {
	var days int64
	rest := s
	if before, after, found := strings.Cut(s, "-"); found {
		d, err := strconv.ParseInt(before, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("procstat: bad etime %q: %w", s, err)
		}
		days = d
		rest = after
	}

	parts := strings.Split(rest, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("procstat: bad etime %q", s)
	}

	var total int64
	for _, part := range parts {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("procstat: bad etime %q: %w", s, err)
		}
		total = total*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(total)*time.Second, nil
}

// FormatRSS renders a byte count compactly: kilobytes below 1 MB, megabytes
// with one decimal below 1 GB, gigabytes with one decimal above.
func FormatRSS(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1fG", float64(bytes)/gb)
	case bytes >= mb:
		return fmt.Sprintf("%.1fM", float64(bytes)/mb)
	default:
		return fmt.Sprintf("%dK", bytes/kb)
	}
}

// FormatUptime renders a duration compactly with its two most significant
// units: 3d04h, 4h12m, 12m05s, or 45s.
func FormatUptime(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		days := d / (24 * time.Hour)
		hours := (d % (24 * time.Hour)) / time.Hour
		return fmt.Sprintf("%dd%02dh", days, hours)
	case d >= time.Hour:
		hours := d / time.Hour
		minutes := (d % time.Hour) / time.Minute
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	case d >= time.Minute:
		minutes := d / time.Minute
		seconds := (d % time.Minute) / time.Second
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", d/time.Second)
	}
}
