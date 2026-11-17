package gather

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func fakeGatectlServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
}

const deviceABody = `# HELP cpu_load1 1-minute CPU load average
# TYPE cpu_load1 gauge
cpu_load1{node_id="device-a"} 0.5
# HELP mem_size_bytes Total memory
# TYPE mem_size_bytes gauge
mem_size_bytes{node_id="device-a"} 100
`

const deviceBBody = `# HELP cpu_load1 1-minute CPU load average
# TYPE cpu_load1 gauge
cpu_load1{node_id="device-b"} 0.8
# HELP uptime_seconds Seconds since the device started
# TYPE uptime_seconds gauge
uptime_seconds{node_id="device-b"} 42
`

func TestDevicesGatherer_MergesFamilies(t *testing.T) {
	a := fakeGatectlServer(t, deviceABody)
	defer a.Close()
	b := fakeGatectlServer(t, deviceBBody)
	defer b.Close()

	g := NewDevicesGatherer([]string{a.URL, b.URL})
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}

	got := map[string]int{}
	for _, fam := range mfs {
		got[fam.GetName()] = len(fam.Metric)
	}

	// cpu_load1 doit être fusionné depuis les deux devices (2 séries),
	// mem_size_bytes seulement depuis A, uptime_seconds seulement depuis B.
	if got["cpu_load1"] != 2 {
		t.Errorf("expected cpu_load1 to have 2 metrics (one per device), got %d", got["cpu_load1"])
	}
	if got["mem_size_bytes"] != 1 {
		t.Errorf("expected mem_size_bytes to have 1 metric, got %d", got["mem_size_bytes"])
	}
	if got["uptime_seconds"] != 1 {
		t.Errorf("expected uptime_seconds to have 1 metric, got %d", got["uptime_seconds"])
	}
}

func TestDevicesGatherer_OneDeviceDown(t *testing.T) {
	a := fakeGatectlServer(t, deviceABody)
	defer a.Close()

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer down.Close()

	g := NewDevicesGatherer([]string{a.URL, down.URL})
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}

	names := []string{}
	for _, fam := range mfs {
		names = append(names, fam.GetName())
	}
	sort.Strings(names)

	if len(names) != 2 || names[0] != "cpu_load1" || names[1] != "mem_size_bytes" {
		t.Errorf("expected only families from device A, got %v", names)
	}
}

func TestLocalReadersGatherer_FallbackOnFailure(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer down.Close()

	g := NewLocalReadersGatherer("regia-test", "terminal-test", down.URL)
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}

	names := map[string]bool{}
	for _, fam := range mfs {
		names[fam.GetName()] = true
	}
	if !names["readers_avail_total"] || !names["readers_total"] {
		t.Errorf("expected fallback families readers_avail_total and readers_total, got %v", names)
	}
}

func TestLocalReadersGatherer_EmptyURL(t *testing.T) {
	g := NewLocalReadersGatherer("regia-test", "terminal-test", "")
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}
	if len(mfs) != 2 {
		t.Errorf("expected 2 fallback families, got %d", len(mfs))
	}
}

func TestLocalReadersGatherer_InjectsNamespaceAndNodeID(t *testing.T) {
	body := `# HELP readers_avail Reader availability
# TYPE readers_avail gauge
readers_avail{reader_id="/dev/ttyUSB0"} 1
# HELP readers_avail_total Number of available readers
# TYPE readers_avail_total gauge
readers_avail_total 1
`
	srv := fakeGatectlServer(t, body)
	defer srv.Close()

	g := NewLocalReadersGatherer("regia-test", "terminal-test", srv.URL)
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}

	want := map[string]string{"node_id": "terminal-test", "namespace": "regia-test"}
	for _, fam := range mfs {
		for _, m := range fam.Metric {
			got := map[string]string{}
			for _, lp := range m.Label {
				got[lp.GetName()] = lp.GetValue()
			}
			for name, value := range want {
				if got[name] != value {
					t.Errorf("expected %s=%s on metric in family %s, labels=%v", name, value, fam.GetName(), m.Label)
				}
			}
		}
	}
}

func TestLocalReadersGatherer_RelaysFamilies(t *testing.T) {
	body := `# HELP readers_avail Reader availability
# TYPE readers_avail gauge
readers_avail{reader_id="/dev/ttyUSB0"} 1
# HELP readers_avail_total Number of available readers
# TYPE readers_avail_total gauge
readers_avail_total 1
# HELP readers_total Total number of declared readers
# TYPE readers_total gauge
readers_total 2
`
	srv := fakeGatectlServer(t, body)
	defer srv.Close()

	g := NewLocalReadersGatherer("regia-test", "terminal-test", srv.URL)
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}

	names := map[string]bool{}
	for _, fam := range mfs {
		names[fam.GetName()] = true
	}
	for _, want := range []string{"readers_avail", "readers_avail_total", "readers_total"} {
		if !names[want] {
			t.Errorf("expected family %s in relayed output, got %v", want, names)
		}
	}
}

func TestDevicesGatherer_SetURLs(t *testing.T) {
	a := fakeGatectlServer(t, deviceABody)
	defer a.Close()
	b := fakeGatectlServer(t, deviceBBody)
	defer b.Close()

	// Démarre avec un seul device, puis bascule sur les deux via SetURLs.
	g := NewDevicesGatherer([]string{a.URL})
	g.SetURLs([]string{a.URL, b.URL, b.URL}) // doublon volontaire

	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather returned error: %v", err)
	}
	got := map[string]int{}
	for _, fam := range mfs {
		got[fam.GetName()] = len(fam.Metric)
	}
	// Doublon de b.URL dédupliqué : cpu_load1 = 2 séries (a + b), pas 3.
	if got["cpu_load1"] != 2 {
		t.Errorf("expected cpu_load1 to have 2 metrics after SetURLs, got %d", got["cpu_load1"])
	}
}
