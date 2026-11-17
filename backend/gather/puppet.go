package gather

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"orbinea.example/core2/utils"
)

var puppetLastUpdateTimestamp = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "puppet_last_update_timestamp_seconds",
		Help: "Last system update check timestamp",
	},
	[]string{"namespace", "node_id"},
)

// Puppet écrit last_run_summary.yaml à la fin de chaque run de l'agent ;
// l'emplacement dépend de la génération/du packaging de l'agent
var puppetLastRunSummaryPaths = []string{
	// Puppet >= 7 : lastrunfile déplacé dans publicdir (world-readable)
	"/opt/puppetlabs/puppet/public/last_run_summary.yaml",
	"/opt/puppetlabs/puppet/cache/state/last_run_summary.yaml",
	"/var/cache/puppet/state/last_run_summary.yaml",
	"/var/lib/puppet/state/last_run_summary.yaml",
}

func RegisterPuppetMetrics() {
	prometheus.MustRegister(puppetLastUpdateTimestamp)
}

func UpdatePuppetMetrics(namespace, nodeID string) {
	puppetLastUpdateTimestamp.WithLabelValues(namespace, nodeID).Set(getPuppetLastRunTime())
}

// getPuppetLastRunTime retourne le timestamp unix du dernier run Puppet, ou 0
// si aucun résumé de run n'est trouvé
func getPuppetLastRunTime() float64 {
	for _, p := range puppetLastRunSummaryPaths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		ts := parsePuppetLastRun(f)
		f.Close()
		if ts == 0 {
			// Le résumé existe mais sans champ last_run ; le fichier étant
			// réécrit à la fin de chaque run, son mtime est une bonne
			// approximation
			if fi, err := os.Stat(p); err == nil {
				ts = float64(fi.ModTime().Unix())
			}
		}
		return ts
	}
	return 0
}

// parsePuppetLastRun extrait le timestamp unix "last_run" d'un document
// last_run_summary.yaml de Puppet
func parsePuppetLastRun(r io.Reader) float64 {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "last_run:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "last_run:"))
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			return v
		}
	}
	return 0
}

func RunPuppet(onOutput func(string)) (err error) {
	cmd := exec.Command("/opt/puppetlabs/bin/puppet", "agent", "-t")
	return utils.RunBashCommand(cmd, onOutput)
}
