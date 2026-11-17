package gather

import (
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestDiscoveryStore_PersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	// Première vie : on découvre deux bornes.
	s1 := newDiscoveryStore(path, 7*24*time.Hour)
	s1.observe("borne-a", "http://10.30.0.1:8193/metrics", now)
	s1.observe("borne-b", "http://10.30.0.2:8193/metrics", now)

	// Redémarrage 2 jours plus tard, les deux bornes éteintes (pas de
	// nouvelle annonce). Elles doivent quand même être rechargées.
	s2 := newDiscoveryStore(path, 7*24*time.Hour)
	got := s2.load(now.Add(2 * 24 * time.Hour))
	sort.Strings(got)
	want := []string{"http://10.30.0.1:8193/metrics", "http://10.30.0.2:8193/metrics"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("load = %v, want %v", got, want)
	}
}

func TestDiscoveryStore_EvictsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	s1 := newDiscoveryStore(path, 7*24*time.Hour)
	s1.observe("vivante", "http://10.30.0.1:8193/metrics", now)
	s1.observe("demontee", "http://10.30.0.9:8193/metrics", now.Add(-10*24*time.Hour))

	// Au reload, la borne non revue depuis 10j (> maxStale 7j) est oubliée.
	s2 := newDiscoveryStore(path, 7*24*time.Hour)
	got := s2.load(now)
	if len(got) != 1 || got[0] != "http://10.30.0.1:8193/metrics" {
		t.Fatalf("load = %v, want only la borne vivante", got)
	}
}

func TestDiscoveryStore_EmptyPathNoFile(t *testing.T) {
	// path vide => persistance désactivée, mais observe/load ne paniquent pas.
	s := newDiscoveryStore("", time.Hour)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	urls := s.observe("x", "http://10.30.0.1/metrics", now)
	if len(urls) != 1 {
		t.Fatalf("observe = %v, want 1 url en mémoire", urls)
	}
	if got := s.load(now); got != nil {
		t.Fatalf("load sur path vide = %v, want nil", got)
	}
}

func TestDiscoveryStore_MissingFile(t *testing.T) {
	// Premier démarrage : fichier absent => liste vide, pas d'erreur.
	s := newDiscoveryStore(filepath.Join(t.TempDir(), "absent.json"), time.Hour)
	if got := s.load(time.Now()); got != nil {
		t.Fatalf("load fichier absent = %v, want nil", got)
	}
}

func TestObserveEvinceLesStales(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	s := newDiscoveryStore(path, 24*time.Hour)

	t0 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	s.observe("borne-a", "http://10.30.0.1:8193/metrics", t0)

	// 25h plus tard, borne-b s'annonce : borne-a (non revue depuis > maxStale)
	// doit être évincée sans attendre un restart
	urls := s.observe("borne-b", "http://10.30.0.2:8193/metrics", t0.Add(25*time.Hour))

	if len(urls) != 1 || urls[0] != "http://10.30.0.2:8193/metrics" {
		t.Errorf("urls = %v, attendu seulement borne-b", urls)
	}
}
