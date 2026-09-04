package user

import (
	"strconv"

	"oneclickvirt/constant"
	"oneclickvirt/global"
	"oneclickvirt/model/common"
	providerModel "oneclickvirt/model/provider"
	upstreamService "oneclickvirt/service/upstream"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UpstreamInstanceAction 代理管理智简魔方上游实例
// 支持动作：start / stop / reboot / reinstall / reset-password / console / delete
// 仅限实例所属用户本人操作，且仅对上游(智简魔方)实例生效。
func UpstreamInstanceAction(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeUnauthorized, err.Error()))
		return
	}

	instanceIDStr := c.Param("id")
	instanceID, err := strconv.ParseUint(instanceIDStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "无效的实例ID"))
		return
	}

	var req struct {
		Action string            `json:"action" binding:"required"`
		OSID   string            `json:"osId"`
		Password string          `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
		return
	}

	// 校验实例归属与上游类型
	var inst providerModel.Instance
	if err := global.APP_DB.First(&inst, uint(instanceID)).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeNotFound, "实例不存在"))
		return
	}
	if inst.UserID != userID {
		common.ResponseWithError(c, common.NewError(common.CodeForbidden, "无权操作该实例"))
		return
	}
	if inst.UpstreamType != constant.UpstreamTypeIDC {
		common.ResponseWithError(c, common.NewError(common.CodeBadRequest, "该实例不是上游(智简魔方)实例"))
		return
	}

	params := map[string]string{}
	if req.OSID != "" {
		params["osId"] = req.OSID
	}
	if req.Password != "" {
		params["password"] = req.Password
	}

	// delete 动作软删实例后，本地记录即消失，故先取回基本信息供前端提示
	result, err := upstreamService.ManageInstance(uint(instanceID), req.Action, params)
	if err != nil {
		global.APP_LOG.Warn("上游实例操作失败",
			zap.Uint("instanceID", uint(instanceID)),
			zap.String("action", req.Action),
			zap.Error(err))
		common.ResponseWithError(c, common.NewError(common.CodeInternalError, err.Error()))
		return
	}

	common.ResponseSuccess(c, result, "操作成功")
}
