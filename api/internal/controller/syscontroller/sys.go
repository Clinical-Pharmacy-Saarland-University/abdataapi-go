package syscontroller

import (
	"net/http"
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
// @Produce		json
// @Success		200	{object}	syscontroller.PingResp	"Response with pong message"
// @Router			/sys/ping [get]
func (sc *SysController) GetPing(c *gin.Context) {
	type PingResponse struct {
		Message string `json:"message" example:"pong"`
	} // @name PingResp

	c.JSON(http.StatusOK, PingResponse{Message: "pong"})
}

// @Summary		Get API Info
// @Description	Get information about the API including version and query limits.
// @Tags			System
// @Produce		json
// @Produce		json
// @Success		200	{object}	syscontroller.InfoResp	"Response with API info"
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

	c.JSON(http.StatusOK, res)
}
