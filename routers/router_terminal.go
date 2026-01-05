package routers

import (
	"github.com/JLPAY/gwayne/controllers/terminal"
	"github.com/JLPAY/gwayne/middleware"
	"github.com/gin-gonic/gin"
)

func SetupTerminalRoutes(rg *gin.RouterGroup) {
	terminalGroup := rg.Group("/terminal").Use(middleware.JWTauth())
	{
		// 命令规则管理
		terminalGroup.GET("/command-rules", terminal.ListCommandRules)
		terminalGroup.GET("/command-rules/:id", terminal.GetCommandRule)
		terminalGroup.POST("/command-rules", terminal.CreateCommandRule)
		terminalGroup.PUT("/command-rules/:id", terminal.UpdateCommandRule)
		terminalGroup.DELETE("/command-rules/:id", terminal.DeleteCommandRule)
		terminalGroup.GET("/command-rules/role/:role", terminal.GetCommandRulesByRole)
	}
}

