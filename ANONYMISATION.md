# Note d'anonymisation

Ce dépôt est la publication d'un travail réalisé pendant un stage de fin de
deuxième année de BUT Informatique (avril – juillet 2026). Le code est
authentique, **mais tous les noms propres ont été remplacés** avant
publication. Rien de ce qui suit ne désigne une entité réelle.

## Ce qui a été remplacé

| Dans ce dépôt | Nature |
|---|---|
| `Orbinea` | l'entreprise d'accueil |
| `Fluxia` | sa branche « équipements » |
| `beacon` | ce service |
| `gatectl` | le logiciel embarqué des tourniquets |
| `core2` | la bibliothèque socle maison |
| `nexus` | la bibliothèque métier centrale |
| `corvette` | la bibliothèque d'accès aux équipements |
| `poshub` | le module caisses |
| `ledger` | le système tiers comparé aux caisses |
| `metropole-a`, `site-alpha` | un client et un site de test |
| les plages `10.30-50.x`, `192.168.10/20/30/40.x` | adressage privé |

Sont authentiques et conservés : Go, Prometheus, OpenMetrics, Grafana, MQTT,
MongoDB, Puppet, zeroconf/mDNS.

## Secret retiré

Une clé d'API était codée en dur dans `backend/poshub/api.go`. Elle a été
remplacée, dans l'arbre et dans le commit, par le marqueur
`REMPLACER_PAR_UNE_VARIABLE_ENV`. Cette clé doit être considérée comme
compromise et révoquée par son propriétaire, indépendamment de cette
publication.

Les fichiers de configuration (`config.dist.json`, `deploy.config.json`) ne
contenaient aucun autre identifiant : les champs de connexion MongoDB et MQTT
y étaient déjà vides.

## Historique

L'historique d'origine vit dans le dépôt interne et n'a pas été rapatrié : ce
dépôt part d'un commit unique, correspondant à l'état du 08/07/2026, après
fusion de la campagne de durcissement. Les binaires compilés (`deployable/`)
ne sont pas publiés.

## Droits

Ce service est un produit interne de l'entreprise d'accueil, que j'ai étendu
pendant mon stage ; l'essentiel de l'ossature n'est pas de moi (voir le
périmètre dans le [README](README.md)). Publié à titre de portfolio, sans
licence d'utilisation : aucun droit de réutilisation n'est concédé.
