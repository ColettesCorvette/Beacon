package main

import (
	"fmt"
	"time"

	"orbinea.example/core2/core"
	mongo "orbinea.example/core2/mongo_v2"
	"orbinea.example/core2/utils"
	"orbinea.example/fluxia/beacon/backend/api"
	"orbinea.example/fluxia/beacon/backend/poshub"
	"orbinea.example/fluxia/beacon/backend/gather"
	"orbinea.example/fluxia/beacon/backend/server"
	central "orbinea.example/fluxia/nexus"
	"orbinea.example/fluxia/nexus/datastore"
)

func main() {
	projectMap := core.GetConfig().Project

	namespace, _ := projectMap["namespace"].(string)
	nodeType, _ := projectMap["node_type"].(string)
	nodeID, _ := projectMap["node_id"].(string)

	if nodeType == "" {
		nodeType = "server"
	}

	if nodeID == "" {
		nodeID = fmt.Sprintf("server-%s", utils.RandString(6))
	}

	kind, _ := projectMap["kind"].(string)
	target, _ := projectMap["ping_target"].(string)

	var deviceURLs []string
	if raw, ok := projectMap["devices"].([]interface{}); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				deviceURLs = append(deviceURLs, s)
			}
		}
	}

	var containers []string
	if raw, ok := projectMap["monitored_containers"].([]interface{}); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				containers = append(containers, s)
			}
		}
	}

	gather.StartMetricsGathering(gather.Config{
		Namespace:  namespace,
		NodeID:     nodeID,
		Kind:       kind,
		PingTarget: target,
		Containers: containers,
	})

	switch nodeType {
	case "server":
		runServerMode(projectMap, nodeID, deviceURLs)
	case "device":
		runDeviceMode(projectMap, namespace, nodeID, kind)
	default:
		panic(fmt.Sprintf("unknown node_type: %q (expected \"server\" or \"device\")", nodeType))
	}

	core.Start()
	core.Standby()
}

func runServerMode(projectMap map[string]interface{}, nodeID string, deviceURLs []string) {
	// Connexion à la base MongoDB
	fmt.Printf("Server mode (node_id=%s)\n", nodeID)

	sess, err := mongo.ConnectDatabaseFromConfig()
	if err != nil {
		panic("cannot connect to mongodb server: " + err.Error())
	}
	mongo.SetDefaultDatabase(sess)

	ds := datastore.NewMongoDatastorePointer(mongo.DefaultDB())

	// Connexion au broker MQTT
	mqcli, err := central.ConnectMQTTFromConfig(nil, func(opts *central.MQClientOptions) { opts.SetCleanSession(true) })
	if err != nil {
		fmt.Printf("Failed to connect to the MQTT server: %s", err)
		panic(err)
	}

	posHub := poshub.New(ds, mqcli)
	err = posHub.Initialize()
	if err != nil {
		fmt.Printf("Failed to initialize cr hub: %s", err)
		panic(err)
	}

	// Mise en place du handler de l'API
	handler := api.NewHandler(posHub, ds, api.Config{
		LocalLedgerAddress:     projectMap["local_ledger_address"].(string),
		LedgerApiKey:           projectMap["ledger_api_key"].(string),
		LocalDatabaseAddress: projectMap["mongo_hosts"].(string),
		LocalDatabaseName:    projectMap["mongo_base"].(string),
	})

	devicesGatherer := gather.NewDevicesGatherer(deviceURLs)

	// Persistance des bornes découvertes en mDNS : sans elle, une borne
	// éteinte au moment d'un redémarrage serait oubliée (off => pas de
	// ré-annonce => HS indétectable). Défauts : fichier à côté de config.json,
	// éviction des bornes non revues depuis 30 jours.
	storePath, ok := projectMap["discovery_store_path"].(string)
	if !ok {
		storePath = "discovered_devices.json"
	}
	maxStaleDays := 30.0
	if v, ok := projectMap["discovery_max_stale_days"].(float64); ok && v > 0 {
		maxStaleDays = v
	}
	maxStale := time.Duration(maxStaleDays * 24 * float64(time.Hour))
	go gather.StartDiscovery(deviceURLs, devicesGatherer, storePath, maxStale)
	s := server.New("server", handler, posHub, devicesGatherer)
	s.Initialize()
}

func runDeviceMode(projectMap map[string]interface{}, namespace, nodeID, kind string) {
	fmt.Printf("Device mode (node_id=%s)\n", nodeID)

	// Le gatherer est construit même sans gatectl_metrics_url : il gère
	// lui-même l'URL vide en exposant les séries readers_* fallback à
	// zéro, pour que Grafana affiche 0 plutôt que NoData
	gatectlURL, _ := projectMap["gatectl_metrics_url"].(string)
	localGatherer := gather.NewLocalReadersGatherer(namespace, nodeID, gatectlURL)

	_, port := server.ResolveListen()
	txt := []string{
		"node_id=" + nodeID,
		"namespace=" + namespace,
		"kind=" + kind,
	}
	if _, err := gather.Advertise("beacon-"+nodeID, port, txt); err != nil {
		fmt.Printf("mdns: annonce impossible: %s\n", err)
	}

	s := server.New("device", nil, nil, localGatherer)
	s.Initialize()
}
