package gather

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var nodeInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "node_info", Help: "Static metadata about this node"},
	[]string{"namespace", "node_id", "kind", "arch", "cpu", "ip_address", "mac", "os", "os_release", "os_version"},
)

func RegisterNodeMetrics() {
	prometheus.MustRegister(nodeInfo)
}

func SetNodeInfo(namespace, nodeID, kind string) {
	cpu := readCPUModel()
	ip, mac := primaryInterface()
	osName, release, version := readSystemOS()
	// La mémoire n'est volontairement plus un label : valeurs dynamiques,
	// déjà exposées par mem_size_bytes / mem_avail_bytes (metrics_system.go)
	nodeInfo.WithLabelValues(namespace, nodeID, kind, runtime.GOARCH, cpu, ip, mac, osName, release, version).Set(1)
}

func readCPUModel() string {
	// "model name" n'existe que sur x86 ; les kernels arm64 ne l'émettent
	// pas. Sur ARM, le modèle de la carte est dans le device-tree, et les
	// vieux kernels arm32 exposent une ligne "Hardware" dans cpuinfo.
	if model := cpuinfoField("model name"); model != "" {
		return model
	}
	if raw, err := os.ReadFile("/proc/device-tree/model"); err == nil {
		// La string device-tree est NUL-terminée
		if model := strings.TrimSpace(strings.Trim(string(raw), "\x00")); model != "" {
			return model
		}
	}
	if model := cpuinfoField("Hardware"); model != "" {
		return model
	}
	return "unknown"
}

// cpuinfoField retourne la valeur du premier champ `key : valeur` de
// /proc/cpuinfo, ou "" si absent
func cpuinfoField(key string) string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, key) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func primaryInterface() (string, string) {
	ip, mac := firstIPv4(func(iface net.Interface) bool {
		// Ignore la loopback et les interfaces down
		return iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0
	})
	if ip == "" {
		return "unknown", "unknown"
	}
	return ip, mac
}

// firstIPv4 parcourt les interfaces réseau et retourne la première adresse
// IPv4 non-loopback (et la MAC) d'une interface satisfaisant keep. Mutualise le
// parcours net.Interfaces entre primaryInterface (node_info) et getEthernetIP
// (API system-status). Retourne "","" si rien ne correspond.
func firstIPv4(keep func(net.Interface) bool) (ip, mac string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	for _, iface := range ifaces {
		if !keep(iface) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ipNet *net.IPNet
			switch v := addr.(type) {
			case *net.IPNet:
				ipNet = v
			case *net.IPAddr:
				ipNet = &net.IPNet{IP: v.IP}
			}
			if ipNet != nil && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
				return ipNet.IP.String(), iface.HardwareAddr.String()
			}
		}
	}
	return "", ""
}

