package httpapi

import "github.com/gin-gonic/gin"

func registerAccountRoutes(api *gin.RouterGroup, accounts AccountUseCases, auth tenantAuthenticator) {
	api.PUT("/account/profile", tenantInput(accounts.Profile))
	api.POST("/account/password", tenantInput(accounts.ChangePassword))
	api.POST("/account/wechat/password", tenantInput(accounts.ChangeWeChatPassword))
	api.POST("/account/avatar", avatarUpload(accounts, auth))
	api.GET("/avatars/:tenant/:user/:name", avatarRead(accounts))
	api.HEAD("/avatars/:tenant/:user/:name", avatarRead(accounts))
	api.GET("/admin/users", endpoint(func(c *gin.Context) (any, error) {
		id, err := identity(c)
		if err != nil {
			return nil, err
		}
		p, err := pageQuery(c)
		if err != nil {
			return nil, err
		}
		return accounts.ListUsers(c.Request.Context(), id, p)
	}))
	api.POST("/admin/users", tenantInput(accounts.CreateUser))
	api.PATCH("/admin/users/:id", tenantTarget(accounts.UpdateUser))
	api.POST("/admin/users/:id/reset-password", tenantTarget(accounts.ResetPassword))
	api.POST("/platform/account/password", platformInput(accounts.PlatformPassword))
}
