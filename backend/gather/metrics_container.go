package gather

import (
	"context"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/prometheus/client_golang/prometheus"
)

var containerStatus = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "container_status",
		Help: "Current state of the container (1=running, 0=stopped/error)",
	},
	[]string{"namespace", "node_id", "container_name"},
)

var containerInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "container_info",
		Help: "Metadata about running services containers",
	},
	[]string{"namespace", "node_id", "container_name", "image_name", "image_tag"},
)

var containerUptimeSeconds = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "container_uptime_seconds",
		Help: "Services containers uptime, in seconds",
	},
	[]string{"namespace", "node_id", "container_name"},
)

func RegisterContainerMetrics() {
	prometheus.MustRegister(containerStatus, containerInfo, containerUptimeSeconds)
}

type containerReading struct {
	name      string
	running   bool
	imageName string
	imageTag  string
	uptime    float64
	ok        bool
}

func UpdateContainerMetrics(namespace, nodeID string, containers []string) {
	// On collecte d'abord tous les inspects (appels API Docker) AVANT de
	// toucher aux gauges. containerInfo.Reset() puis la repopulation se font
	// ensuite sans I/O entre les deux : la fenêtre où un scrape /metrics
	// verrait container_info vide ou partiel se réduit à quelques Set mémoire.
	readings := make([]containerReading, 0, len(containers))
	for _, name := range containers {
		running, imageName, imageTag, uptime, err := inspectContainer(name)
		if err != nil {
			log.Errorf("failed to inspect container %s: %s", name, err)
			readings = append(readings, containerReading{name: name, ok: false})
			continue
		}
		readings = append(readings, containerReading{name, running, imageName, imageTag, uptime, true})
	}

	containerInfo.Reset()
	for _, r := range readings {
		if r.ok && r.running {
			containerStatus.WithLabelValues(namespace, nodeID, r.name).Set(1)
			containerUptimeSeconds.WithLabelValues(namespace, nodeID, r.name).Set(r.uptime)
		} else {
			containerStatus.WithLabelValues(namespace, nodeID, r.name).Set(0)
			containerUptimeSeconds.WithLabelValues(namespace, nodeID, r.name).Set(0)
		}
		if r.ok {
			containerInfo.WithLabelValues(namespace, nodeID, r.name, r.imageName, r.imageTag).Set(1)
		}
	}
}

func inspectContainer(name string) (running bool, imageName, imageTag string, uptime float64, err error) {
	ctx := context.Background()

	cli, err := dockerClient()
	if err != nil {
		return
	}

	resp, err := cli.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		return
	}

	c := resp.Container
	running = c.State != nil && c.State.Running

	if c.Config != nil {
		imageName, imageTag = parseImageRef(c.Config.Image)
	}

	// StartedAt, pas Created : un conteneur redémarré repart de son dernier
	// démarrage, pas de sa date de création
	if running && c.State != nil && c.State.StartedAt != "" {
		if t, perr := time.Parse(time.RFC3339Nano, c.State.StartedAt); perr == nil {
			uptime = time.Since(t).Seconds()
		}
	}
	return
}

func parseImageRef(ref string) (name, tag string) {
	tag = "latest"
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if colon > slash {
		name = ref[:colon]
		tag = ref[colon+1:]
	} else {
		name = ref
	}
	return
}
