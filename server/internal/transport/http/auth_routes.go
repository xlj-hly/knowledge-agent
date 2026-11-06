package httpapi

import "github.com/gin-gonic/gin"

func registerAuthRoutes(api *gin.RouterGroup, auth AuthUseCases) {
	api.GET("/tenant", tenantRead(auth.Tenant))
	api.POST("/auth/login", tenantInput(auth.Login))
	api.POST("/auth/wechat/login", tenantInput(auth.WeChatLogin))
	api.GET("/auth/me", tenantRead(auth.Me))
	api.POST("/auth/logout", tenantRead(auth.Logout))
	api.POST("/platform/auth/login", input(auth.PlatformLogin))
	api.GET("/platform/auth/me", platformRead(auth.PlatformMe))
	api.POST("/platform/auth/logout", platformRead(auth.PlatformLogout))
}
