# beacon — exporter OpenMetrics pour équipements isolés

Service Go qui expose l'état d'un parc d'équipements (tourniquets, bornes)
installés chez des clients, sous une forme que Prometheus peut collecter.

## Le problème

L'infrastructure est en oignon. Les serveurs centraux joignent, par VPN, un
serveur local installé chez le client ; derrière ce serveur vivent les
équipements, sur un réseau qui n'est pas routable depuis l'extérieur. Seul le
serveur local est atteignable. Superviser les équipements suppose donc que
quelque chose, à l'intérieur, aille chercher leur état et le republie.

## La solution

`beacon` tourne déjà sur les serveurs locaux. Il devient un **exporter
OpenMetrics 1.0 à deux modes**, choisis par `config.json` (généré par Puppet) :

- **mode `server`** — expose ses propres métriques et **agrège** celles des
  équipements listés dans `devices`, via un collecteur maison
  (`DevicesGatherer`). Il découvre aussi les équipements en mDNS.
- **mode `device`** — tourne sur l'équipement, expose ses métriques et
  **relaie** celles de `gatectl` (le logiciel embarqué) en y injectant les
  étiquettes `node_id` et `namespace`.

Un Prometheus central ne voit alors qu'un seul point de collecte par site,
branché sur Grafana.

## Métriques

| Famille | Exemples |
|---|---|
| système | `mem_*`, `cpu_load*`, `fs_*`, `uptime` |
| conteneurs | `container_status`, `container_info`, `container_uptime` |
| réseau | `net_pings_*` |
| thermique | `cpu_temp_*` |
| identité | `node_info` |
| lecteurs | `readers_*`, dont `readers_use_total` (persistant sur disque) |

## Périmètre et limites

**Ce dépôt ne compile pas en l'état.** Le service fait partie d'un monorepo
interne : `go.mod` pointe par `replace` vers une dizaine de modules maison
(`orbinea.example/core2/*`, `orbinea.example/fluxia/nexus`) qui ne sont pas
publiés. Il est ici comme **pièce de lecture**, pas comme projet exécutable.

Tout le code n'est pas de moi. Ma contribution porte sur la collecte de
métriques — le paquet `backend/gather`, la découverte mDNS, le capteur
thermique — et sur la campagne de durcissement documentée dans
`docs/superpowers/plans/`. Les paquets `backend/api`, `backend/server`,
`backend/poshub` et l'ossature du service préexistaient.

La mise en production a été assurée par un collègue ; de mon côté le travail
a été implémenté puis validé en local.

Voir [ANONYMISATION.md](ANONYMISATION.md) — les noms ont été remplacés.
