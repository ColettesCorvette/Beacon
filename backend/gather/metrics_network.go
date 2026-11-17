package gather

import (
	"os/exec"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var netPingsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{Name: "net_pings_total", Help: "Total number of pings"},
	[]string{"namespace", "node_id", "target"},
)
var netPingsFailedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{Name: "net_pings_failed_total", Help: "Total number of failed pings"},
	[]string{"namespace", "node_id", "target"},
)
var netPingsLatencySeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{Name: "net_pings_latency_seconds", Help: "Ping latency in seconds", Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5}},
	[]string{"namespace", "node_id", "target"},
)

func RegisterNetworkMetrics() {
	prometheus.MustRegister(netPingsTotal, netPingsFailedTotal, netPingsLatencySeconds)
}

func StartNetworkPings(namespace, nodeID, target string) {
	if target == "" {
		return
	}

	netPingsFailedTotal.WithLabelValues(namespace, nodeID, target).Add(0)

	go func() {
		for {
			for i := 0; i < 10; i++ {
				doPing(namespace, nodeID, target)
				time.Sleep(time.Second)
			}
			time.Sleep(50 * time.Second)
		}
	}()
}

func doPing(namespace, nodeID, target string) {
	start := time.Now()
	cmd := exec.Command("ping", "-c", "1", "-W", "2", target)
	err := cmd.Run()
	elapsed := time.Since(start).Seconds()
	netPingsTotal.WithLabelValues(namespace, nodeID, target).Inc()
	if err != nil {
		netPingsFailedTotal.WithLabelValues(namespace, nodeID, target).Inc()
		return
	}
	netPingsLatencySeconds.WithLabelValues(namespace, nodeID, target).Observe(elapsed)
}
