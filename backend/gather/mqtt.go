package gather

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	central "orbinea.example/fluxia/nexus"
)

func getPersistenceDatabasePath() string {
	possibilities := []string{
		"/var/lib/docker-volumes/mosquitto/data/mosquitto.db",
		"/data/docker-volumes/mosquitto/data/mosquitto.db",
		"/var/lib/mosquitto/mosquitto.db",
	}

	for _, p := range possibilities {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "/path/notfound"
}

func GetPersistenceDatabaseSize() int64 {
	fi, err := os.Stat(getPersistenceDatabasePath())
	if err != nil {
		log.Errorf("Failed to find mosquitto database: %s", err)
		return 0
	}

	return fi.Size()
}

func GetRemoteMQTTPersistenceDatabaseMetadata() (central.LHPersistenceMetadata, error) {
	// timeout : ce dump est appelé depuis un handler HTTP, un mosquitto_db_dump
	// qui bloque ne doit pas bloquer l'API indéfiniment
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/local/sbin/mosquitto_db_dump", getPersistenceDatabasePath())

	content, err := cmd.Output()
	if err != nil {
		return central.LHPersistenceMetadata{}, errors.Wrap(err, "failed to run db dump")
	}
	if len(content) == 0 {
		return central.LHPersistenceMetadata{}, fmt.Errorf("empty mosquitto db dump, probably bad right for persistance db file %s", getPersistenceDatabasePath())
	}

	return parsePersistenceDump(bytes.NewReader(content)), nil
}

func parsePersistenceDump(r io.Reader) (res central.LHPersistenceMetadata) {
	scan := bufio.NewScanner(r)

	chunk := ""
	metaCli := &central.LHPersistenceMetadataClient{}
	metaMsg := &central.LHPersistenceMetadataMessage{}
	metaCliMsg := &central.LHPersistenceMetadataClientMessage{}
	metaSub := &central.LHPersistenceMetadataSub{}

	for scan.Scan() {
		line := scan.Text()

		if strings.HasPrefix(line, "Mosquitto DB") || strings.HasPrefix(line, "CRC: ") ||
			strings.HasPrefix(line, "DB version: ") {
			continue
		} else if line == "DB_CHUNK_CFG:" {
			chunk = "CFG"
		} else if line == "DB_CHUNK_MSG_STORE:" {
			chunk = "MSG_STORE"
			metaMsg = &central.LHPersistenceMetadataMessage{}
			res.Msg = append(res.Msg, metaMsg)
		} else if line == "DB_CHUNK_CLIENT_MSG:" {
			chunk = "CLIENT_MSG"
			metaCliMsg = &central.LHPersistenceMetadataClientMessage{}
			res.CliMsg = append(res.CliMsg, metaCliMsg)
		} else if line == "DB_CHUNK_CLIENT:" {
			chunk = "CLIENT"
			metaCli = &central.LHPersistenceMetadataClient{}
			res.Cli = append(res.Cli, metaCli)
		} else if line == "DB_CHUNK_SUB:" {
			chunk = "SUB"
			metaSub = &central.LHPersistenceMetadataSub{}
			res.Sub = append(res.Sub, metaSub)
		} else if line == "DB_CHUNK_RETAIN:" {
			chunk = "RETAIN"
		} else if !strings.HasPrefix(line, "\t") {
			log.Warningf("Unknown chunk type: %s", line)
		} else {
			keyPair := strings.TrimSpace(line)
			if !strings.Contains(keyPair, ": ") {
				log.Warningf("Odd keyPair: %s", keyPair)
				continue
			}

			// SplitN : un topic peut contenir ": ", seul le premier séparateur
			// délimite la clé
			keyPairSlice := strings.SplitN(keyPair, ": ", 2)
			if len(keyPairSlice) < 2 {
				log.Warningf("Odd keyPair: %s", keyPair)
				continue
			}

			key := keyPairSlice[0]
			val := keyPairSlice[1]

			if chunk == "CFG" || chunk == "RETAIN" {
				// ignore
			} else if chunk == "CLIENT" {
				if key == "Client ID" {
					metaCli.ID = val
				} else if key == "Session expiry time" {
					metaCli.ExpTs, _ = strconv.ParseUint(val, 10, 64)
				} else if key == "Session expiry interval" {
					metaCli.ExpItv, _ = strconv.ParseUint(val, 10, 64)
				}
			} else if chunk == "MSG_STORE" {
				if key == "Payload Length" {
					metaMsg.Sz, _ = strconv.ParseUint(val, 10, 64)
				} else if key == "Store ID" {
					metaMsg.ID = val
				} else if key == "QoS" {
					metaMsg.QoS, _ = strconv.Atoi(val)
				} else if key == "Topic" {
					metaMsg.Topic = val
				}
			} else if chunk == "CLIENT_MSG" {
				if key == "Client ID" {
					metaCliMsg.Cli = val
				} else if key == "Store ID" {
					metaCliMsg.Msg = val
				}
			} else if chunk == "SUB" {
				if key == "Client ID" {
					metaSub.CliID = val
				} else if key == "Topic" {
					metaSub.Topic = val
				} else if key == "QoS" {
					metaSub.QoS, _ = strconv.Atoi(val)
				}
			}
		}
	}

	return res
}
