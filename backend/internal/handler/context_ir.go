package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
	"yingce/backend/internal/service"
)

func RegisterContextIRRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.POST("/context-ir/jobs", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if !enforceRateLimit(c, "context-ir:"+user.ID, 4, time.Minute) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
		var req service.IRPromptRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			failService(c, service.BadAuthRequest("IR 请求格式无效"))
			return
		}
		job, err := svc.CreateIRPrompt(user.ID, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, job)
	})
	r.GET("/context-ir/jobs/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		job, err := svc.IRPrompt(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, job)
	})
}
