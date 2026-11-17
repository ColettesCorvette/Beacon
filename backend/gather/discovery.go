package gather

import (
	"context"
	"fmt"
	"time"

	"github.com/grandcat/zeroconf"
)

const mdnsService = "_beacon._tcp"
const mdnsDomain = "local."

// entryURL construit l'URL /metrics d'un device découvert en mDNS. Retourne
// "" si l'entrée n'a pas d'adresse IPv4 ou pas de port utilisable.
// Les devices IPv6-only sont volontairement ignorés (LAN de site en IPv4).
func entryURL(e *zeroconf.ServiceEntry) string {
	if len(e.AddrIPv4) == 0 || e.Port == 0 {
		return ""
	}
	return fmt.Sprintf("http://%s:%d/metrics", e.AddrIPv4[0].String(), e.Port)
}

// mergeURLs fusionne les devices statiques (config.json) et découverts (mDNS),
// en dédupliquant.
func mergeURLs(static, discovered []string) []string {
	all := make([]string, 0, len(static)+len(discovered))
	all = append(all, static...)
	all = append(all, discovered...)
	return dedupURLs(all)
}

// Advertise publie un service mDNS _beacon._tcp pour ce device, afin que
// le mode server le découvre automatiquement. Le *zeroconf.Server retourné
// doit rester vivant pendant toute la durée du process (ne pas appeler
// Shutdown).
func Advertise(instance string, port uint16, txt []string) (*zeroconf.Server, error) {
	return zeroconf.Register(instance, mdnsService, mdnsDomain, int(port), txt, nil)
}

// StartDiscovery recharge les bornes persistées (store disque, filtrées par
// maxStale) + la liste statique, seede le gatherer, puis lance en arrière-plan
// le browse mDNS. À chaque annonce reçue, la borne est rafraîchie dans le store
// et la liste fusionnée (statique + découvert) est poussée dans le gatherer.
// config.json n'est jamais touché ; seul le store (storePath) est écrit.
//
// La persistance corrige le trou de la découverte in-memory : une borne
// éteinte au moment d'un redémarrage est rechargée depuis le store, donc
// toujours scrapée (le scrape échoue proprement) et son absence reste visible.
// L'éviction par maxStale évite qu'une borne réellement démontée reste scrapée
// (et donc « HS ») éternellement. storePath vide => persistance désactivée.
func StartDiscovery(static []string, g *DevicesGatherer, storePath string, maxStale time.Duration) {
	store := newDiscoveryStore(storePath, maxStale)
	persisted := store.load(time.Now())
	g.SetURLs(mergeURLs(static, persisted))
	if len(persisted) > 0 {
		log.Infof("mdns: %d borne(s) rechargée(s) depuis %s", len(persisted), storePath)
	}

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Errorf("mdns: création du resolver impossible: %s", err)
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for e := range entries {
			url := entryURL(e)
			if url == "" {
				continue
			}
			urls := store.observe(e.Instance, url, time.Now())
			g.SetURLs(mergeURLs(static, urls))
			log.Infof("mdns: %d device(s) découvert(s)", len(urls))
		}
	}()

	if err := resolver.Browse(context.Background(), mdnsService, mdnsDomain, entries); err != nil {
		log.Errorf("mdns: browse impossible: %s", err)
	}
}
