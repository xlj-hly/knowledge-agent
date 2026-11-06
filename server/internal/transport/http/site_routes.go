package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func registerSiteRoutes(r *gin.Engine, sites SiteUseCases, maintenance MaintenanceUseCases) {
	r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })
	r.GET("/api/health", endpoint(func(c *gin.Context) (any, error) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		return maintenance.Health(ctx)
	}))
	r.GET("/api/site", endpoint(func(c *gin.Context) (any, error) {
		return sites.Site(c.Request.Context(), c.Request.Host)
	}))
}
