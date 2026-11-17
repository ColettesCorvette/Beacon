package gather

import (
	"time"
)

type Config struct {
	Namespace  string
	NodeID     string
	Kind       string
	PingTarget string
	Containers []string
}

func StartMetricsGathering(cfg Config) {
	RegisterSystemMetrics()
	RegisterContainerMetrics()
	RegisterNetworkMetrics()
	RegisterNodeMetrics()
	RegisterPuppetMetrics()
	RegisterThermalMetrics()
	SetNodeInfo(cfg.Namespace, cfg.NodeID, cfg.Kind)
	StartNetworkPings(cfg.Namespace, cfg.NodeID, cfg.PingTarget)

	go func() {
		for {
			UpdateSystemMetrics(cfg.Namespace, cfg.NodeID)
			UpdateContainerMetrics(cfg.Namespace, cfg.NodeID, cfg.Containers)
			UpdatePuppetMetrics(cfg.Namespace, cfg.NodeID)
			UpdateThermalMetrics(cfg.Namespace, cfg.NodeID)
			time.Sleep(10 * time.Second)
		}
	}()
}
