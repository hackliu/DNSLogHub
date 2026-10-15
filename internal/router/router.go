package router

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hackliu/DNSLogHub/internal/config"
	"github.com/hackliu/DNSLogHub/internal/handler"
	"github.com/hackliu/DNSLogHub/internal/middleware"
)

//go:embed resources/template/*.html
var tmplFS embed.FS

//go:embed resources/js/*.js
var jsFS embed.FS

//go:embed resources/css/*.css
var cssFS embed.FS

func SetupRouter() *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())

	if !config.Config.Http.ConsoleDisable {
		tmpl := template.Must(template.New("").Delims("[[", "]]").ParseFS(tmplFS, "resources/template/*.html"))
		r.SetHTMLTemplate(tmpl)
		jsFiles, _ := fs.Sub(jsFS, "resources/js")
		r.StaticFS("/js", http.FS(jsFiles))
		cssFiles, _ := fs.Sub(cssFS, "resources/css")
		r.StaticFS("/css", http.FS(cssFiles))

		console := func(c *gin.Context) { c.HTML(http.StatusOK, "index.html", gin.H{}) }
		if config.Config.Register.Enabled {
			// 开启注册时，首页进入欢迎页，可跳转注册/控制台
			r.GET("/", func(c *gin.Context) { c.HTML(http.StatusOK, "welcome.html", gin.H{}) })
			r.GET("/welcome", func(c *gin.Context) { c.HTML(http.StatusOK, "welcome.html", gin.H{}) })
			r.GET("/register", func(c *gin.Context) { c.HTML(http.StatusOK, "register.html", gin.H{}) })
			r.GET("/console", console)
		} else {
			r.GET("/", console)
		}
		r.GET("/docs", func(c *gin.Context) { c.HTML(http.StatusOK, "docs.html", gin.H{}) })
		adminLoginPath := config.Config.Admin.LoginPath
		if adminLoginPath == "" {
			adminLoginPath = "/admin"
		}
		adminDashboardPath := config.Config.Admin.DashboardPath
		if adminDashboardPath == "" {
			adminDashboardPath = "/admin/dashboard"
		}
		r.GET(adminLoginPath, func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin_login.html", gin.H{
				"LoginPath":     adminLoginPath,
				"DashboardPath": adminDashboardPath,
			})
		})
		r.GET(adminDashboardPath, func(c *gin.Context) {
			c.HTML(http.StatusOK, "admin.html", gin.H{
				"LoginPath":     adminLoginPath,
				"DashboardPath": adminDashboardPath,
			})
		})
	}

	api := r.Group("/api")

	// 公开接口
	api.Any("/verifyToken", handler.VerifyToken)
	api.POST("/register", handler.Register)

	// IP查询接口
	api.GET("/ip", handler.Query)
	api.GET("/ip/:ip", handler.Query)
	api.GET("/domains", handler.Domains)

	// 需要鉴权的接口
	authApi := api.Group("/", middleware.AuthMiddleware())
	{
		authApi.Any("/getDnsData", handler.GetDnsData)
		authApi.Any("/clean", handler.Clean)
		authApi.Any("/getDnsData_clear", handler.GetDnsDataAndClean)
		authApi.Any("/verifyDns", handler.VerifyDns)
		authApi.Any("/bulkVerifyDns", handler.BulkVerifyDns)
		authApi.Any("/verifyHttp", handler.VerifyHttp)
		authApi.Any("/bulkVerifyHttp", handler.BulkVerifyHttp)
		authApi.Any("/setARecord", handler.SetARecord)
		authApi.Any("/setTXTRecord", handler.SetTXTRecord)
	}

	// 管理接口（需管理员令牌）
	admin := api.Group("/admin", middleware.AdminAuth())
	{
		admin.GET("/ping", handler.Ping)
		admin.POST("/reload", handler.Reload)
		admin.GET("/invites", handler.ListInvites)
		admin.POST("/invites/revoke", handler.RevokeInvite)
		admin.GET("/users", handler.ListUsers)
		admin.POST("/users/disable", handler.DisableUser)
		admin.POST("/users/enable", handler.EnableUser)
		admin.POST("/users/reset-subdomain", handler.AdminResetSubdomain)
	}

	// 用户自助管理（需登录 token）
	authApi.POST("/user/reset-subdomain", handler.UserResetSubdomain)
	authApi.POST("/user/reset-token", handler.UserResetToken)

	r.NoRoute(handler.NoRoute)
	return r
}
