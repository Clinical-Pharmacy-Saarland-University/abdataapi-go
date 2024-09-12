package syscontroller

import (
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/handle"

	"github.com/gin-gonic/gin"
)

type SysController struct {
	Meta  cfg.MetaConfig
	Limit cfg.LimitsConfig
}

func NewSysController(resourceHandle *handle.ResourceHandle) *SysController {
	return &SysController{
		Meta:  resourceHandle.MetaCfg,
		Limit: resourceHandle.Limits,
	}
}

// @Summary		Ping the API
// @Description	Ping the API to check if it is alive.
// @Tags			System
// @Produce		json
// @Success		200	{object}	handle.jsendSuccess[syscontroller.PingResp]	"Response with pong message"
// @Router			/sys/ping [get]
func (sc *SysController) GetPing(c *gin.Context) {
	type PingResponse struct {
		Message string `json:"message" example:"pong"` // Message
	} // @name PingResp

	handle.Success(c, PingResponse{Message: "pong"})
}

// @Summary		Get API Info
// @Description	Get information about the API including version and query limits.
// @Tags			System
// @Produce		json
// @Success		200	{object}	handle.jsendSuccess[syscontroller.InfoResp]	"Response with API info"
// @router			/sys/info [get]
func (sc *SysController) GetInfo(c *gin.Context) {
	type InfoResponse struct {
		API    cfg.MetaConfig   `json:"meta_info"`  // Meta
		Limits cfg.LimitsConfig `json:"api_limits"` // Limits
	} // @name InfoResp

	res := InfoResponse{
		API:    sc.Meta,
		Limits: sc.Limit,
	}

	handle.Success(c, res)
}
