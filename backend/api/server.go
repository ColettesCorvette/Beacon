package api

import (
	"orbinea.example/core2/http"
	"orbinea.example/core2/websocket"
	"orbinea.example/fluxia/beacon/backend/gather"
	"os"
	"strings"
)

func (api *Handler) GetServerSystemStatus(c *http.Context) {
	status, err := gather.GetServerSystemStatus()
	if err != nil {
		log.Errorf("Failed to get server system status: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, status)
}

// updateFactsContent remplace la ligne `key=…` dans un contenu facts.conf,
// ou l'ajoute en fin si absente. Match strict sur `key=` (pas HasPrefix nu :
// la clé "role" ne doit pas matcher "roleplay").
func updateFactsContent(content, key, value string) string {
	var out []string
	replaced := false
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, key+"=") {
			out = append(out, key+"="+value)
			replaced = true
			continue
		}
		out = append(out, line)
	}
	if !replaced {
		out = append(out, key+"="+value)
	}
	return strings.Join(out, "\n") + "\n"
}

func (api *Handler) SetFact(c *http.Context) {
	type Fact struct {
		Key   string
		Value string
	}

	var fact Fact
	if err := c.ParseJSON(&fact); err != nil {
		log.Errorf("Failed to parse fact request: %s", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	// clé/valeur injectées dans un fichier ligne-par-ligne : refuser tout
	// séparateur qui permettrait d'écrire une autre ligne ou une autre clé
	if fact.Key == "" || strings.ContainsAny(fact.Key, "=\n") || strings.Contains(fact.Value, "\n") {
		log.Errorf("Invalid fact key/value: key=%q", fact.Key)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	factsConf, err := os.ReadFile("/etc/facts.conf")
	if err != nil {
		log.Errorf("Failed to open facts.conf: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	updated := updateFactsContent(string(factsConf), fact.Key, fact.Value)
	// Écriture atomique (tmp + rename), même pattern que discovery_store :
	// un crash mi-écriture ne doit jamais tronquer un fichier consommé par Puppet
	tmp := "/etc/facts.conf.tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o644); err != nil {
		log.Errorf("Failed to write facts.conf: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmp, "/etc/facts.conf"); err != nil {
		log.Errorf("Failed to replace facts.conf: %s", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, nil)
}

func (api *Handler) RunPuppet(c *http.Context) {
	go func() {
		err := gather.RunPuppet(func(s string) {
			api.sendWS(websocket.GenericWSFrame{
				Channel: "notify:puppet-run-progress",
				Data:    s,
			})
		})
		if err != nil {
			log.Errorf("Failed to run puppet: %s", err)
			api.sendWS(websocket.GenericWSFrame{
				Channel: "notify:puppet-run-err",
				Data:    err.Error(), // err brut se sérialise en "{}"
			})
		}
		api.sendWS(websocket.GenericWSFrame{Channel: "notify:puppet-run-done"})
	}()

	c.JSON(http.StatusOK, nil)
}
