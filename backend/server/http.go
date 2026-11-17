package server

import (
	"crypto/subtle"
	"os"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"orbinea.example/fluxia/beacon/backend/api"
	"orbinea.example/fluxia/beacon/backend/poshub"

	"orbinea.example/core2/core"
	"orbinea.example/core2/http"
	"orbinea.example/core2/http/middleware"
)

type Server struct {
	nodeType      string
	handler       *api.Handler
	poshub         *poshub.PosHub
	httpServer    *http.Server
	extraGatherer prometheus.Gatherer
}

// ResolveListen retourne l'adresse et le port d'écoute, en appliquant la
// config puis les surcharges d'environnement LISTENADDR / LISTENPORT.
func ResolveListen() (string, uint16) {
	listenPort := 8193.0
	if v, ok := core.GetConfig().Project["listen_port"].(float64); ok {
		listenPort = v
	}
	listenAddr := "0.0.0.0"
	if v, ok := core.GetConfig().Project["listen_addr"].(string); ok && v != "" {
		listenAddr = v
	}
	if os.Getenv("LISTENPORT") != "" {
		if p, err := strconv.ParseFloat(os.Getenv("LISTENPORT"), 64); err == nil {
			listenPort = p
		}
	}
	if os.Getenv("LISTENADDR") != "" {
		listenAddr = os.Getenv("LISTENADDR")
	}
	return listenAddr, uint16(listenPort)
}

// apiKeyMiddleware exige Authorization: <key> sur les routes du groupe.
// Rétro-compatible : n'est monté que si api_key est définie en config.
func apiKeyMiddleware(key string) http.HandlerFunc {
	return func(c *http.Context) {
		// Comparaison constant-time : une comparaison != fuit la position
		// du premier octet différent via le temps de réponse
		got := []byte(c.Request.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, []byte(key)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func New(nodeType string, handler *api.Handler, poshub *poshub.PosHub, extraGatherer prometheus.Gatherer) *Server {
	listenAddr, listenPort := ResolveListen()

	httpServer := http.NewHttpServer(http.Config{
		Port:          listenPort,
		ListenAddress: listenAddr,
	})

	return &Server{nodeType: nodeType, handler: handler, poshub: poshub, httpServer: httpServer, extraGatherer: extraGatherer}
}

func (s *Server) Initialize() {
	s.SetupRoutes()

	// Le proxy cashregisters et le hub WebSocket dépendent de poshub et de
	// handler, qui sont nil en mode device : ne les monter qu'en mode
	// server, sinon toute requête /cashregisters ou /ws ferait paniquer
	// le processus
	if s.nodeType == "server" {
		s.SetupCashregistersProxy()
	}

	// Register our HTTP server service. We can have multiple at once.
	core.RegisterService(s.httpServer)
}

func (s *Server) SetupRoutes() {

	// Log HTTP requests
	// s.httpServer.Use(middleware.HttpLoggingMiddleware)
	s.httpServer.Use(middleware.AllowAllOriginsMiddleware)

	// Fallback to index.html if the actual file wasn't found
	s.httpServer.RouteNotFound(func(c *http.Context) {
		c.SetHeader("Access-Control-Allow-Origin", "*")
		c.SetHeader("Access-Control-Allow-Headers", "*")
		c.SetHeader("Access-Control-Allow-Methods", "*")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
		} else {
			c.AbortWithStatus(http.StatusNotFound)
		}
	})

	// /metrics combine les métriques locales (DefaultGatherer) avec le
	// gatherer extra du mode courant : agrégation des devices en mode
	// server, relais Gatectl en mode device
	var gatherer prometheus.Gatherer = prometheus.DefaultGatherer
	if s.extraGatherer != nil {
		gatherer = prometheus.Gatherers{prometheus.DefaultGatherer, s.extraGatherer}
	}
	metrics := s.httpServer.Group("/metrics")
	metrics.Mount(promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))

	if s.nodeType == "server" {
		// Routes de l'API de supervision (mode server uniquement)
		// /api/ping reste ouvert (health check), hors du groupe protégé
		s.httpServer.GET("/api/ping", s.handler.GetPing)

		apiGroup := s.httpServer.Group("/api")
		apiKey, _ := core.GetConfig().Project["api_key"].(string)
		if apiKey != "" {
			apiGroup.Use(apiKeyMiddleware(apiKey))
		} else {
			log.Warningf("api_key absente de config.json : l'API de supervision est SANS authentification")
		}

		apiGroup.GET("/server/system-status", s.handler.GetServerSystemStatus)
		apiGroup.GET("/docker/container/:name", s.handler.GetDockerContainer)
		apiGroup.GET("/broker/persistence/size", s.handler.GetBrokerPersistenceSize)
		apiGroup.GET("/broker/persistence/metadata", s.handler.GetBrokerPersistenceMetadata)
		apiGroup.POST("/broker/persistence/update", s.handler.PostBrokerPersistenceUpdate)
		apiGroup.GET("/cashregisters/health", s.handler.GetCashregistersHealth)
		apiGroup.PUT("/compare", s.handler.Compare)
		apiGroup.PUT("/puppet/run", s.handler.RunPuppet)
		apiGroup.PUT("/puppet/set-fact", s.handler.SetFact)
	}

}
