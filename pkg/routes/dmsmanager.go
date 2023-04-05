package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/lamassuiot/lamassuiot/pkg/config"
	"github.com/lamassuiot/lamassuiot/pkg/controllers"
	"github.com/lamassuiot/lamassuiot/pkg/models"
	"github.com/lamassuiot/lamassuiot/pkg/services"
)

func NewDMSManagerHTTPLayer(svc services.DMSManagerService, httpServerCfg config.HttpServer, apiInfo models.APIServiceInfo) error {
	if !httpServerCfg.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(ginResponseErorrLogger, gin.Logger(), gin.Recovery())

	routes := controllers.NewDMSManagerHttpRoutes(svc)

	NewESTHttpRoutes(router, svc)

	rv1 := router.Group("/v1")

	rv1.GET("/dms", routes.GetAllDMSs)
	rv1.POST("/dms", routes.CreateDMS)
	rv1.GET("/dms/:id", routes.GetDMSByID)
	rv1.PUT("/dms/:id/status", routes.UpdateStatus)
	rv1.PUT("/dms/:id/id-profile", routes.UpdateIdentityProfile)

	return newHttpRouter(router, httpServerCfg, apiInfo)
}
