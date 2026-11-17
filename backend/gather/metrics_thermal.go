package gather

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
)

var cpuTemp = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "cpu_temp_celsius", Help: "Current CPU temperature, in Celsius"},
	[]string{"namespace", "node_id", "sensor", "label"},
)

var cpuTempMax = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "cpu_temp_max_celsius", Help: "Critical CPU temperature (Tjmax), in Celsius"},
	[]string{"namespace", "node_id", "sensor", "label"},
)

var cpuTempMargin = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "cpu_temp_margin_celsius", Help: "Margin to critical CPU temperature (Tjmax minus current), in Celsius"},
	[]string{"namespace", "node_id", "sensor", "label"},
)

var cpuThrottle = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{Name: "cpu_thermal_throttle_total", Help: "Cumulative thermal throttling events since boot (from kernel log)"},
	[]string{"namespace", "node_id"},
)

const (
	hwmonRoot   = "/sys/class/hwmon"
	thermalRoot = "/sys/class/thermal"
	kmsgPath    = "/dev/kmsg"
)

// cpuSensorNames : noms de drivers hwmon considérés comme capteurs CPU. On
// ignore le reste (acpitz, pch_*, nvme, etc.) : la métrique est cpu_temp, et
// les capteurs disque/chipset exposent en plus des _max sentinelles aberrantes
// (ex. nvme temp*_max ~ 65000 °C).
var cpuSensorNames = map[string]bool{
	"coretemp":    true, // Intel
	"k10temp":     true, // AMD (Zen/K10)
	"k8temp":      true, // AMD K8
	"zenpower":    true, // AMD Zen (module tiers)
	"cpu_thermal": true, // ARM (Raspberry Pi & co)
	"soc_thermal": true, // ARM Rockchip (RK33xx : capteur CPU = SoC, GPU séparé)
}

// isCPUThermalZone ne garde que les zones thermiques liées au CPU (ARM
// "cpu-thermal" / Rockchip "soc-thermal", x86 "x86_pkg_temp"), pas les zones
// GPU/ambiantes. "soc" matche soc-thermal mais pas gpu-thermal.
func isCPUThermalZone(t string) bool {
	t = strings.ToLower(t)
	return strings.Contains(t, "cpu") || strings.Contains(t, "soc") || strings.Contains(t, "x86_pkg")
}

func RegisterThermalMetrics() {
	prometheus.MustRegister(cpuTemp, cpuTempMax, cpuTempMargin, cpuThrottle)
}

func UpdateThermalMetrics(namespace, nodeID string) {
	for _, r := range readSensorsFromDir(hwmonRoot, thermalRoot) {
		cpuTemp.WithLabelValues(namespace, nodeID, r.Sensor, r.Label).Set(r.Temp)
		if r.HasMax {
			cpuTempMax.WithLabelValues(namespace, nodeID, r.Sensor, r.Label).Set(r.Max)
			cpuTempMargin.WithLabelValues(namespace, nodeID, r.Sensor, r.Label).Set(r.Max - r.Temp)
		}
	}
	if count, ok := readThrottleFromKmsg(); ok {
		cpuThrottle.WithLabelValues(namespace, nodeID).Set(count)
	}
}

// readThrottleFromKmsg ouvre /dev/kmsg en non-bloquant (sinon la lecture
// bloque en attente de nouveaux messages après avoir vidé le buffer) et en
// extrait le compteur de throttling. Échoue silencieusement si le fichier est
// illisible (non-root ou kernel.dmesg_restrict=1).
func readThrottleFromKmsg() (float64, bool) {
	f, err := os.OpenFile(kmsgPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	return readThrottleCount(f)
}

type thermalReading struct {
	Sensor string
	Label  string
	Temp   float64
	Max    float64
	HasMax bool
}

// parseTempMilli convertit une valeur sysfs en millidegrés ("45000\n") en
// degrés Celsius (45.0).
func parseTempMilli(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v / 1000.0, true
}

var throttleRe = regexp.MustCompile(`total events = (\d+)`)

// parseThrottleCount extrait le compteur cumulatif d'une ligne kmsg de
// throttling thermique coretemp.
func parseThrottleCount(line string) (float64, bool) {
	m := throttleRe.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// readThrottleCount scanne un flux kmsg et retourne le dernier compteur de
// throttling thermique rencontré (le noyau émet un cumulatif croissant).
func readThrottleCount(r io.Reader) (float64, bool) {
	scanner := bufio.NewScanner(r)
	var last float64
	found := false
	for scanner.Scan() {
		if v, ok := parseThrottleCount(scanner.Text()); ok {
			last = v
			found = true
		}
	}
	return last, found
}

func readFileTrim(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// readSensorsFromDir lit les capteurs de température sous hwmonRoot
// (/sys/class/hwmon). Si aucun capteur hwmon n'est trouvé, retombe sur
// thermalRoot (/sys/class/thermal) sans Tjmax.
func readSensorsFromDir(hwmonRoot, thermalRoot string) []thermalReading {
	readings := readHwmon(hwmonRoot)
	if len(readings) > 0 {
		return readings
	}
	return readThermalZones(thermalRoot)
}

func readHwmon(hwmonRoot string) []thermalReading {
	devices, _ := filepath.Glob(filepath.Join(hwmonRoot, "hwmon*"))
	var out []thermalReading
	for _, dev := range devices {
		sensor, _ := readFileTrim(filepath.Join(dev, "name"))
		if !cpuSensorNames[sensor] {
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(dev, "temp*_input"))
		for _, input := range inputs {
			raw, ok := readFileTrim(input)
			if !ok {
				continue
			}
			temp, ok := parseTempMilli(raw)
			if !ok {
				continue
			}
			prefix := strings.TrimSuffix(input, "_input") // .../tempN
			label, _ := readFileTrim(prefix + "_label")
			r := thermalReading{Sensor: sensor, Label: label, Temp: temp}
			// Tjmax : _crit en priorité, sinon _max.
			for _, suffix := range []string{"_crit", "_max"} {
				if mraw, ok := readFileTrim(prefix + suffix); ok {
					if m, ok := parseTempMilli(mraw); ok {
						r.Max = m
						r.HasMax = true
						break
					}
				}
			}
			out = append(out, r)
		}
	}
	return out
}

func readThermalZones(thermalRoot string) []thermalReading {
	zones, _ := filepath.Glob(filepath.Join(thermalRoot, "thermal_zone*"))
	var out []thermalReading
	for _, zone := range zones {
		raw, ok := readFileTrim(filepath.Join(zone, "temp"))
		if !ok {
			continue
		}
		temp, ok := parseTempMilli(raw)
		if !ok {
			continue
		}
		sensor, _ := readFileTrim(filepath.Join(zone, "type"))
		if !isCPUThermalZone(sensor) {
			continue
		}
		out = append(out, thermalReading{Sensor: sensor, Temp: temp})
	}
	return out
}
