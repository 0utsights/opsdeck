package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ContainerInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Health string `json:"health"`
}

type Probe struct {
	Hostname       string          `json:"hostname"`
	Arch           string          `json:"arch"`
	CPUPercent     float64         `json:"cpu_percent"`
	MemoryPercent  float64         `json:"memory_percent"`
	MemoryUsed     uint64          `json:"memory_used"`
	MemoryTotal    uint64          `json:"memory_total"`
	DiskPercent    float64         `json:"disk_percent"`
	DiskUsed       uint64          `json:"disk_used"`
	DiskTotal      uint64          `json:"disk_total"`
	Load1          float64         `json:"load_1"`
	NetRxBytesSec  float64         `json:"net_rx_bytes_sec"`
	NetTxBytesSec  float64         `json:"net_tx_bytes_sec"`
	UptimeSeconds  float64         `json:"uptime_seconds"`
	Containers     []ContainerInfo `json:"containers"`
	CollectedAtUTC time.Time       `json:"collected_at_utc"`
}

type ServerState struct {
	Config      ServerConfig
	Probe       Probe
	Online      bool
	Latency     time.Duration
	HTTPStatus  int
	Error       string
	CPUHistory  []float64
	MemHistory  []float64
	LastUpdated time.Time
}

type AgentState struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Task      string    `json:"task"`
	Model     string    `json:"model"`
	PID       int       `json:"pid"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkflowFile struct {
	Items []WorkflowItem `json:"items"`
}

type WorkflowItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Done    bool   `json:"done"`
	Owner   string `json:"owner,omitempty"`
	Status  string `json:"status,omitempty"`
	Created string `json:"created,omitempty"`
}

func printProbe() error {
	p, err := collectLocalProbe()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(p)
}

func collectLocalProbe() (Probe, error) {
	var p Probe
	host, _ := os.Hostname()
	p.Hostname, p.Arch = host, runtime.GOARCH
	cpu1, _ := readCPU()
	rx1, tx1 := readNetwork()
	t0 := time.Now()
	time.Sleep(220 * time.Millisecond)
	cpu2, _ := readCPU()
	rx2, tx2 := readNetwork()
	elapsed := time.Since(t0).Seconds()
	if total := float64(cpu2.total - cpu1.total); total > 0 {
		p.CPUPercent = 100 * (1 - float64(cpu2.idle-cpu1.idle)/total)
	}
	p.NetRxBytesSec = float64(rx2-rx1) / elapsed
	p.NetTxBytesSec = float64(tx2-tx1) / elapsed
	p.MemoryUsed, p.MemoryTotal = readMemory()
	if p.MemoryTotal > 0 {
		p.MemoryPercent = 100 * float64(p.MemoryUsed) / float64(p.MemoryTotal)
	}
	p.DiskUsed, p.DiskTotal = readDisk()
	if p.DiskTotal > 0 {
		p.DiskPercent = 100 * float64(p.DiskUsed) / float64(p.DiskTotal)
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		fmt.Sscan(string(b), &p.Load1)
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		fmt.Sscan(string(b), &p.UptimeSeconds)
	}
	p.Containers = readContainers()
	p.CollectedAtUTC = time.Now().UTC()
	return p, nil
}

type cpuStat struct{ idle, total uint64 }

func readCPU() (cpuStat, error) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuStat{}, err
	}
	line := strings.SplitN(string(b), "\n", 2)[0]
	f := strings.Fields(line)
	var vals []uint64
	for _, s := range f[1:] {
		v, _ := strconv.ParseUint(s, 10, 64)
		vals = append(vals, v)
	}
	var total uint64
	for _, v := range vals {
		total += v
	}
	var idle uint64
	if len(vals) > 3 {
		idle = vals[3]
	}
	if len(vals) > 4 {
		idle += vals[4]
	}
	return cpuStat{idle: idle, total: total}, nil
}

func readMemory() (uint64, uint64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	vals := map[string]uint64{}
	s := bufio.NewScanner(strings.NewReader(string(b)))
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) >= 2 {
			vals[strings.TrimSuffix(f[0], ":")], _ = strconv.ParseUint(f[1], 10, 64)
		}
	}
	total, available := vals["MemTotal"]*1024, vals["MemAvailable"]*1024
	return total - available, total
}

func readNetwork() (uint64, uint64) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	var rx, tx uint64
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) >= 9 {
			a, _ := strconv.ParseUint(f[0], 10, 64)
			b, _ := strconv.ParseUint(f[8], 10, 64)
			rx += a
			tx += b
		}
	}
	return rx, tx
}

func readContainers() []ContainerInfo {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}|{{.Status}}")
	b, err := cmd.Output()
	if err != nil {
		return nil
	}
	var out []ContainerInfo
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		c := ContainerInfo{Name: parts[0]}
		if len(parts) > 1 {
			c.Status = parts[1]
			s := strings.ToLower(parts[1])
			switch {
			case strings.Contains(s, "unhealthy"):
				c.Health = "unhealthy"
			case strings.Contains(s, "healthy"):
				c.Health = "healthy"
			default:
				c.Health = "running"
			}
		}
		out = append(out, c)
	}
	return out
}

func collectServer(ctx context.Context, cfg ServerConfig) ServerState {
	state := ServerState{Config: cfg}
	start := time.Now()
	switch cfg.Kind {
	case "local":
		p, err := collectLocalProbe()
		state.Latency = time.Since(start)
		if err != nil {
			state.Error = err.Error()
			return state
		}
		state.Probe, state.Online = p, true
	case "ssh":
		args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=4"}
		if cfg.IdentityFile != "" {
			args = append(args, "-i", cfg.IdentityFile)
		}
		target := cfg.Address
		if cfg.User != "" {
			target = cfg.User + "@" + cfg.Address
		}
		args = append(args, target)
		if cfg.Sudo {
			args = append(args, "sudo", "-n")
		}
		args = append(args, cfg.ProbePath, "--probe")
		cctx, cancel := context.WithTimeout(ctx, 7*time.Second)
		defer cancel()
		b, err := exec.CommandContext(cctx, "ssh", args...).Output()
		state.Latency = time.Since(start)
		if err != nil {
			state.Error = err.Error()
			return state
		}
		if err := json.Unmarshal(b, &state.Probe); err != nil {
			state.Error = err.Error()
			return state
		}
		state.Online = true
	case "http":
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(cctx, http.MethodGet, cfg.URL, nil)
		resp, err := (&http.Client{}).Do(req)
		state.Latency = time.Since(start)
		if err != nil {
			state.Error = err.Error()
			return state
		}
		resp.Body.Close()
		state.HTTPStatus = resp.StatusCode
		state.Online = resp.StatusCode >= 200 && resp.StatusCode < 500
		state.Probe.CollectedAtUTC = time.Now().UTC()
	default:
		state.Error = "unknown server kind"
	}
	state.LastUpdated = time.Now()
	return state
}

func loadAgents(dir string) []AgentState {
	var agents []AgentState
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var a AgentState
		if json.Unmarshal(b, &a) == nil {
			agents = append(agents, a)
		}
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents
}

func loadWorkflows(path string) WorkflowFile {
	var wf WorkflowFile
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, &wf)
	}
	return wf
}

func saveWorkflows(path string, wf WorkflowFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func staleAgent(a AgentState) bool {
	return a.UpdatedAt.IsZero() || time.Since(a.UpdatedAt) > 90*time.Second
}

func ensureDir(path string) error {
	return os.MkdirAll(path, fs.FileMode(0700))
}
