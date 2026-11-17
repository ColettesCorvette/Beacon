package gather

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
)

type fsInfo struct {
	mountpoint string
	fsType     string
	size       float64
	avail      float64
}

var memAvail = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "mem_avail_bytes",
		Help: "Available memory, in bytes",
	},
	[]string{"namespace", "node_id"},
)

var memSize = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "mem_size_bytes", Help: "Total memory, in bytes"},
	[]string{"namespace", "node_id"},
)

var uptimeSeconds = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "uptime_seconds", Help: "Seconds since the device started"},
	[]string{"namespace", "node_id"},
)

var cpuLoad1 = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "cpu_load1", Help: "1-minute CPU load average"},
	[]string{"namespace", "node_id"},
)
var cpuLoad5 = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "cpu_load5", Help: "5-minutes CPU load average"},
	[]string{"namespace", "node_id"},
)
var cpuLoad15 = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "cpu_load15", Help: "15-minutes CPU load average"},
	[]string{"namespace", "node_id"},
)

var fsAvailBytes = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "fs_avail_bytes", Help: "Available disk space of mount point, in bytes"},
	[]string{"namespace", "node_id", "mountpoint", "fs_type"},
)
var fsSizeBytes = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "fs_size_bytes", Help: "Total disk space of mount point, in bytes"},
	[]string{"namespace", "node_id", "mountpoint", "fs_type"},
)

var keptFsTypes = map[string]bool{
	"ext4":  true,
	"ext3":  true,
	"ext2":  true,
	"xfs":   true,
	"btrfs": true,
	"vfat":  true,
	"exfat": true,
	"f2fs":  true,
}

func RegisterSystemMetrics() {
	prometheus.MustRegister(memAvail, memSize, uptimeSeconds,
		cpuLoad1, cpuLoad5, cpuLoad15,
		fsAvailBytes, fsSizeBytes)
}

func UpdateSystemMetrics(namespace, nodeID string) {

	available, total := getMemInfo()
	memAvail.WithLabelValues(namespace, nodeID).Set(available)
	memSize.WithLabelValues(namespace, nodeID).Set(total)

	uptimeSeconds.WithLabelValues(namespace, nodeID).Set(getUptime())

	l1, l5, l15 := getLoadAvg()
	cpuLoad1.WithLabelValues(namespace, nodeID).Set(l1)
	cpuLoad5.WithLabelValues(namespace, nodeID).Set(l5)
	cpuLoad15.WithLabelValues(namespace, nodeID).Set(l15)

	// Collecte d'abord, Reset ensuite : réduit la fenêtre où un scrape
	// verrait les séries vides (même pattern que UpdateContainerMetrics).
	// Sans Reset, un montage disparu laisse des séries fantômes.
	fsList := getFsInfo()
	fsAvailBytes.Reset()
	fsSizeBytes.Reset()
	for _, fs := range fsList {
		fsAvailBytes.WithLabelValues(namespace, nodeID, fs.mountpoint, fs.fsType).Set(fs.avail)
		fsSizeBytes.WithLabelValues(namespace, nodeID, fs.mountpoint, fs.fsType).Set(fs.size)
	}
}

func getMemInfo() (available, total float64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	found := 0
	for scanner.Scan() && found < 2 {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			if v, err := strconv.ParseFloat(fields[1], 64); err == nil {
				total = v * 1024
				found++
			}
		case "MemAvailable:":
			if v, err := strconv.ParseFloat(fields[1], 64); err == nil {
				available = v * 1024
				found++
			}
		}
	}
	return
}

func getUptime() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}

func getLoadAvg() (load1, load5, load15 float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	load1, _ = strconv.ParseFloat(fields[0], 64)
	load5, _ = strconv.ParseFloat(fields[1], 64)
	load15, _ = strconv.ParseFloat(fields[2], 64)
	return
}

func getFsInfo() []fsInfo {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()

	seen := make(map[string]bool)
	var result []fsInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		mountpoint, fsType := fields[1], fields[2]
		if !keptFsTypes[fsType] {
			continue
		}
		if seen[mountpoint] {
			continue
		}
		seen[mountpoint] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountpoint, &stat); err != nil {
			continue
		}
		bsize := float64(stat.Bsize)
		result = append(result, fsInfo{
			mountpoint: mountpoint,
			fsType:     fsType,
			size:       float64(stat.Blocks) * bsize,
			avail:      float64(stat.Bavail) * bsize,
		})
	}
	return result
}
