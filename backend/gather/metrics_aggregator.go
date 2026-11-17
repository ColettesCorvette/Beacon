package gather

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

type DevicesGatherer struct {
	mu     sync.RWMutex
	urls   []string
	Client *http.Client
}

func NewDevicesGatherer(urls []string) *DevicesGatherer {
	return &DevicesGatherer{
		urls:   dedupURLs(urls),
		Client: &http.Client{Timeout: 5 * time.Second},
	}
}

// dedupURLs retire les URLs en double : une même URL scrapée deux fois
// produirait des séries dupliquées et promhttp répondrait 500 sur tout
// le /metrics.
func dedupURLs(urls []string) []string {
	seen := map[string]bool{}
	deduped := make([]string, 0, len(urls))
	for _, u := range urls {
		if seen[u] {
			log.Warningf("devices: URL %s dupliquée, ignorée", u)
			continue
		}
		seen[u] = true
		deduped = append(deduped, u)
	}
	return deduped
}

// SetURLs remplace la liste des devices à scraper. Appelé par la goroutine
// de découverte mDNS pendant que Gather peut tourner dans le handler /metrics.
func (g *DevicesGatherer) SetURLs(urls []string) {
	deduped := dedupURLs(urls)
	g.mu.Lock()
	g.urls = deduped
	g.mu.Unlock()
}

func (g *DevicesGatherer) snapshotURLs() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]string, len(g.urls))
	copy(out, g.urls)
	return out
}

func (g *DevicesGatherer) Gather() ([]*dto.MetricFamily, error) {
	// Scrape tous les devices en parallèle : Gather est appelé dans le
	// handler /metrics, et des scrapes séquentiels (timeout 5 s chacun)
	// feraient dépasser le scrape_timeout de Prometheus dès que quelques
	// devices sont injoignables
	urls := g.snapshotURLs()
	results := make([]map[string]*dto.MetricFamily, len(urls))
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			families, err := scrapeOne(g.Client, url)
			if err != nil {
				log.Errorf("scrape %s: %s", url, err)
				return
			}
			results[i] = families
		}(i, url)
	}
	wg.Wait()

	merged := map[string]*dto.MetricFamily{}
	for _, families := range results {
		for name, fam := range families {
			// Écarte les familles runtime Go/process : elles sont émises
			// par les collecteurs Prometheus par défaut côté serveur ET
			// côté device avec des labels identiques, ce qui ferait
			// échouer promhttp sur des séries dupliquées dans /metrics
			if isRuntimeFamily(name) {
				continue
			}
			if existing, ok := merged[name]; ok {
				existing.Metric = append(existing.Metric, fam.Metric...)
			} else {
				merged[name] = fam
			}
		}
	}

	out := make([]*dto.MetricFamily, 0, len(merged))
	for _, fam := range merged {
		out = append(out, dropDuplicateSeries(fam))
	}
	return out, nil
}

// isRuntimeFamily indique si la famille vient des collecteurs runtime par
// défaut de client_golang (go_*, process_*)
func isRuntimeFamily(name string) bool {
	return strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_")
}

// dropDuplicateSeries retire d'une famille les métriques dont le jeu de
// labels est déjà présent : deux devices configurés avec le même node_id
// suffiraient sinon à mettre tout le /metrics du serveur en erreur
func dropDuplicateSeries(fam *dto.MetricFamily) *dto.MetricFamily {
	seen := map[string]bool{}
	kept := fam.Metric[:0]
	for _, m := range fam.Metric {
		key := labelsKey(m)
		if seen[key] {
			log.Warningf("famille %s : série dupliquée (%s), ignorée", fam.GetName(), key)
			continue
		}
		seen[key] = true
		kept = append(kept, m)
	}
	fam.Metric = kept
	return fam
}

func labelsKey(m *dto.Metric) string {
	pairs := make([]string, 0, len(m.Label))
	for _, lp := range m.Label {
		pairs = append(pairs, lp.GetName()+"="+lp.GetValue())
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func scrapeOne(client *http.Client, url string) (map[string]*dto.MetricFamily, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	// Le scheme de validation doit être fixé explicitement : le parser à
	// zéro-value panique (voir commit 6002cbc)
	parser := expfmt.NewTextParser(model.UTF8Validation)
	return parser.TextToMetricFamilies(resp.Body)
}

// LocalReadersGatherer scrape le /metrics de la Gatectl locale et relaie
// ses familles en injectant node_id sur chaque métrique. Si le scrape
// échoue, expose des séries fallback à zéro pour que Grafana affiche 0
// plutôt que NoData.
type LocalReadersGatherer struct {
	Namespace string
	NodeID    string
	URL       string
	Client    *http.Client
}

func NewLocalReadersGatherer(namespace, nodeID, url string) *LocalReadersGatherer {
	return &LocalReadersGatherer{
		Namespace: namespace,
		NodeID:    nodeID,
		URL:       url,
		Client:    &http.Client{Timeout: 2 * time.Second},
	}
}

// injectIdentity ajoute namespace et node_id aux familles relayées : la
// Gatectl n'émet ni l'un ni l'autre, et toutes les séries beacon
// portent ces deux labels (les dashboards filtrent dessus)
func (g *LocalReadersGatherer) injectIdentity(families []*dto.MetricFamily) []*dto.MetricFamily {
	return injectLabel(injectLabel(families, "namespace", g.Namespace), "node_id", g.NodeID)
}

func (g *LocalReadersGatherer) Gather() ([]*dto.MetricFamily, error) {
	if g.URL == "" {
		return g.injectIdentity(fallbackReadersFamilies()), nil
	}
	families, err := scrapeOne(g.Client, g.URL)
	if err != nil {
		log.Errorf("scrape gatectl %s: %s", g.URL, err)
		return g.injectIdentity(fallbackReadersFamilies()), nil
	}
	out := make([]*dto.MetricFamily, 0, len(families))
	for name, fam := range families {
		// Même filtre que DevicesGatherer : la Gatectl est aussi un
		// exporter Go, et relayer ses familles go_*/process_* entrerait en
		// conflit avec celles du collecteur par défaut de ce binaire
		if isRuntimeFamily(name) {
			continue
		}
		out = append(out, fam)
	}
	return g.injectIdentity(out), nil
}

// injectLabel ajoute un label à chaque métrique de chaque famille.
// Les métriques qui portent déjà ce label sont laissées telles quelles
func injectLabel(families []*dto.MetricFamily, name, value string) []*dto.MetricFamily {
	if value == "" {
		return families
	}
	for _, fam := range families {
		for _, m := range fam.Metric {
			already := false
			for _, lp := range m.Label {
				if lp.GetName() == name {
					already = true
					break
				}
			}
			if already {
				continue
			}
			n, v := name, value
			m.Label = append(m.Label, &dto.LabelPair{Name: &n, Value: &v})
		}
	}
	return families
}

func fallbackReadersFamilies() []*dto.MetricFamily {
	gauge := dto.MetricType_GAUGE
	zero := float64(0)
	mk := func(name, help string) *dto.MetricFamily {
		n := name
		h := help
		return &dto.MetricFamily{
			Name: &n,
			Help: &h,
			Type: &gauge,
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: &zero}},
			},
		}
	}
	return []*dto.MetricFamily{
		mk("readers_avail_total", "Number of available readers"),
		mk("readers_total", "Total number of declared readers"),
	}
}
