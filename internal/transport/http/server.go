package httptransport

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zhanghaiyang/adflow/internal/health"
	"github.com/zhanghaiyang/adflow/internal/observability"
)

type RouteRegistrar interface {
	RegisterRoutes(*gin.RouterGroup)
}

func NewServer(addr, environment string, logger *slog.Logger, healthService *health.Service, metrics *observability.Metrics, registrars ...RouteRegistrar) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           NewRouter(environment, logger, healthService, metrics, registrars...),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func NewRouter(environment string, logger *slog.Logger, healthService *health.Service, metrics *observability.Metrics, registrars ...RouteRegistrar) *gin.Engine {
	if environment != "local" && environment != "test" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(
		RequestID(),
		CORS(environment),
		metrics.Middleware(),
		AccessLogger(logger),
		Recovery(logger),
	)

	healthHandler := NewHealthHandler(healthService)
	router.GET("/livez", healthHandler.Live)
	router.GET("/readyz", healthHandler.Ready)
	router.GET("/metrics", gin.WrapH(metrics.Handler()))
	api := router.Group("/v1")
	for _, registrar := range registrars {
		registrar.RegisterRoutes(api)
	}
	router.NoRoute(func(c *gin.Context) {
		RespondError(c, http.StatusNotFound, "not_found", "route not found", nil)
	})
	router.NoMethod(func(c *gin.Context) {
		RespondError(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	})
	return router
}
