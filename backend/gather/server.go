package gather

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	central "orbinea.example/fluxia/nexus"
)

func GetServerSystemStatus() (central.LHServerSystemStatus, error) {
	out := central.LHServerSystemStatus{}
	bootTime, err := getLastSystemBootTime()
	if err != nil {
		return out, err
	}

	out.Boot = bootTime
	out.CPUUsage = getCPUUsage(1)
	out.DiskTotal, out.DiskFree = diskSpace()
	out.EthernetIP = getEthernetIP()
	out.BrokerPersistSize = GetPersistenceDatabaseSize()

	return out, nil
}

// getEthernetIP retourne l'IPv4 de la première interface ethernet (eth*/en*),
// via le parcours mutualisé firstIPv4. "0.0.0.0" si aucune.
func getEthernetIP() string {
	ip, _ := firstIPv4(func(i net.Interface) bool {
		return strings.HasPrefix(i.Name, "eth") || strings.HasPrefix(i.Name, "en")
	})
	if ip == "" {
		return "0.0.0.0"
	}
	return ip
}

// diskSpace retourne l'espace total et disponible du système de fichiers du
// répertoire courant, en octets. Un seul Statfs au lieu de deux.
func diskSpace() (total, free uint64) {
	var stat syscall.Statfs_t
	wd, _ := os.Getwd()
	if err := syscall.Statfs(wd, &stat); err != nil {
		return 0, 0
	}
	bsize := uint64(stat.Bsize)
	return stat.Blocks * bsize, stat.Bavail * bsize
}

// getLastSystemBootTime dérive l'heure de boot de /proc/uptime (via
// getUptime, metrics_system.go) : pas de locale, pas de sous-process,
// pas de panic — l'ancien parsing de `who -b` cassait hors locale en/fr.
func getLastSystemBootTime() (time.Time, error) {
	up := getUptime()
	if up == 0 {
		return time.Time{}, fmt.Errorf("uptime indisponible (/proc/uptime)")
	}
	return time.Now().Add(-time.Duration(up * float64(time.Second))), nil
}

func getCPUUsage(sampleSecs uint32) float64 {
	idle0, total0 := getCPUSample()
	time.Sleep(time.Duration(sampleSecs) * time.Second)
	idle1, total1 := getCPUSample()

	idleTicks := float64(idle1 - idle0)
	totalTicks := float64(total1 - total0)
	if totalTicks <= 0 {
		return 0
	}
	return 100 * (totalTicks - idleTicks) / totalTicks
}

func getCPUSample() (idle, total uint64) {
	contents, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	return parseCPUStat(string(contents))
}

// parseCPUStat extrait idle et total de la ligne agrégée "cpu" de /proc/stat.
// Tolère contenu vide et lignes vides (retourne 0,0).
func parseCPUStat(content string) (idle, total uint64) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		for i := 1; i < len(fields); i++ {
			val, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				continue
			}
			total += val
			if i == 4 { // idle est le 5e champ
				idle = val
			}
		}
		return
	}
	return
}
