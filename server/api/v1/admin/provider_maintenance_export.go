package admin

import (
	"strconv"

	"oneclickvirt/model/common"

	"github.com/gin-gonic/gin"
)

// BuildInstanceVNCInfoForUser is used by the user API package without duplicating VNC resolution code.
func BuildInstanceVNCInfoForUser(instanceID uint, userID uint) (gin.H, error) {
	return buildInstanceVNCInfo(instanceID, userID, false)
}

func ProxyInstanceVNCForUser(c *gin.Context, instanceID uint, userID uint) {
	host, port, err := resolveInstanceVNCTarget(instanceID, userID, false)
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}
	proxyVNCWebSocket(c, host, port)
}

// AdminInstanceVNCInfo returns whether WebVNC is available for an admin request.
func AdminInstanceVNCInfo(c *gin.Context) {
	instanceID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "无效的实例ID"))
		return
	}
	info, err := buildInstanceVNCInfo(uint(instanceID), 0, true)
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}
	common.ResponseSuccess(c, info)
}

// AdminInstanceVNCWebSocket proxies a VNC TCP stream to WebSocket for admin (noVNC).
func AdminInstanceVNCWebSocket(c *gin.Context) {
	instanceID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "无效的实例ID"))
		return
	}
	host, port, err := resolveInstanceVNCTarget(uint(instanceID), 0, true)
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}
	proxyVNCWebSocket(c, host, port)
}
