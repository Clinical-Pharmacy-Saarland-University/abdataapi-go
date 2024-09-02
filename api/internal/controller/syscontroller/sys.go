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
		Meta: resourceHandle.MetaCfg,
	}
}

func (sc *SysController) GetPing(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

func (sc *SysController) GetInfo(c *gin.Context) {

	res := struct {
		API    cfg.MetaConfig   `json:"meta_info"`
		Limits cfg.LimitsConfig `json:"api_limits"`
	}{
		sc.Meta,
		sc.Limit,
	}

	c.JSON(http.StatusOK, res)
}
