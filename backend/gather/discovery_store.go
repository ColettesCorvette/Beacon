package gather

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// discoveredDevice est une borne vue en mDNS, persistée pour survivre à un
// redémarrage du process. LastSeen sert d'horloge d'éviction : une borne non
// revue depuis maxStale est présumée démontée et oubliée au prochain load.
type discoveredDevice struct {
	Instance string    `json:"instance"`
	URL      string    `json:"url"`
	LastSeen time.Time `json:"last_seen"`
}

// discoveryStore garde en mémoire les bornes découvertes et les persiste sur
// disque, pour que le mode server n'oublie pas une borne éteinte au moment d'un
// redémarrage (sinon, off => pas de ré-annonce => HS indétectable). Un path
// vide désactive la persistance (tout reste en mémoire, comportement legacy).
type discoveryStore struct {
	path     string
	maxStale time.Duration
	mu       sync.Mutex
	devices  map[string]discoveredDevice // instance -> device
}

func newDiscoveryStore(path string, maxStale time.Duration) *discoveryStore {
	return &discoveryStore{
		path:     path,
		maxStale: maxStale,
		devices:  map[string]discoveredDevice{},
	}
}

// load lit le fichier et conserve les bornes vues depuis moins de maxStale,
// puis retourne leurs URLs. Fichier absent ou illisible => liste vide sans
// erreur fatale (premier démarrage, ou store corrompu qu'on régénère).
func (s *discoveryStore) load(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil // absent au premier démarrage
	}
	var list []discoveredDevice
	if err := json.Unmarshal(raw, &list); err != nil {
		log.Warningf("discovery store: %s illisible, ignoré: %s", s.path, err)
		return nil
	}
	urls := make([]string, 0, len(list))
	for _, d := range list {
		if now.Sub(d.LastSeen) >= s.maxStale {
			continue // borne présumée démontée
		}
		s.devices[d.Instance] = d
		urls = append(urls, d.URL)
	}
	return urls
}

// observe enregistre/rafraîchit une borne découverte, persiste, et retourne
// l'ensemble des URLs connues.
// ponytail: écrit le fichier à chaque annonce. Un site a <~20 bornes et le
// JSON fait quelques Ko ; négligeable. Si ça churn un jour, débattre les
// écritures (ne persister que si lastSeen a bougé de > maxStale/10).
func (s *discoveryStore) observe(instance, url string, now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.devices[instance] = discoveredDevice{Instance: instance, URL: url, LastSeen: now}
	urls := make([]string, 0, len(s.devices))
	for k, d := range s.devices {
		// Éviction en continu, pas seulement au load() : sans ça, un serveur
		// qui tourne des mois scrape à vie une borne démontée. Plafond : sans
		// aucune annonce mDNS, pas d'éviction (observe jamais appelé).
		if now.Sub(d.LastSeen) >= s.maxStale {
			delete(s.devices, k)
			continue
		}
		urls = append(urls, d.URL)
	}
	if err := s.persist(); err != nil {
		log.Errorf("discovery store: écriture %s impossible: %s", s.path, err)
	}
	return urls
}

// persist écrit le store de façon atomique (tmp + rename) pour ne jamais
// laisser un fichier tronqué si le process meurt en cours d'écriture.
// Suppose s.mu déjà tenu.
func (s *discoveryStore) persist() error {
	if s.path == "" {
		return nil
	}
	list := make([]discoveredDevice, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, d)
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
