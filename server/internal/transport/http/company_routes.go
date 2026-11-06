package httpapi

import "github.com/gin-gonic/gin"

func registerCompanyRoutes(api *gin.RouterGroup, companies CompanyUseCases) {
	api.PATCH("/admin/tenant", tenantInput(companies.UpdateOwnTenant))
	api.GET("/platform/tenants", endpoint(func(c *gin.Context) (any, error) {
		token, err := bearer(c)
		if err != nil {
			return nil, err
		}
		p, err := pageQuery(c)
		if err != nil {
			return nil, err
		}
		return companies.ListTenants(c.Request.Context(), token, p)
	}))
	api.POST("/platform/tenants", platformInput(companies.CreateTenant))
	api.GET("/platform/tenants/:id/wechat", endpoint(func(c *gin.Context) (any, error) {
		token, err := bearer(c)
		if err != nil {
			return nil, err
		}
		return companies.WeChatConfig(c.Request.Context(), token, c.Param("id"))
	}))
	api.PUT("/platform/tenants/:id/wechat", platformTarget(companies.SaveWeChatConfig))
	api.PATCH("/platform/tenants/:id", platformTarget(companies.UpdateTenant))
	api.POST("/platform/tenants/:id/reset-admin-password", platformTarget(companies.ResetAdminPassword))
}
