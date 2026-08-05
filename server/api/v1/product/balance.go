package product

import (
	"oneclickvirt/global"
	"oneclickvirt/middleware"
	"oneclickvirt/model/common"
	productModel "oneclickvirt/model/product"
	userModel "oneclickvirt/model/user"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GetUserBalance 获取用户余额
// @Summary 获取用户余额
// @Description 获取当前登录用户的账户余额及总消费金额
// @Tags 用户余额
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} common.Response{data=object} "获取成功"
// @Failure 401 {object} common.Response "用户未登录"
// @Failure 500 {object} common.Response "获取失败"
// @Router /user/balance [get]
func GetUserBalance(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeUnauthorized, err.Error()))
		return
	}

	var user userModel.User
	if err := global.APP_DB.Select("id, balance").First(&user, userID).Error; err != nil {
		global.APP_LOG.Error("获取用户余额失败", zap.Uint("userID", userID), zap.Error(err))
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	// 计算总消费金额（type为consume的记录中amount为负数，取绝对值求和）
	var totalConsumption float64
	if err := global.APP_DB.Model(&productModel.UserBalanceLog{}).
		Where("user_id = ? AND type = ?", userID, "consume").
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&totalConsumption).Error; err != nil {
		global.APP_LOG.Error("获取用户总消费失败", zap.Uint("userID", userID), zap.Error(err))
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, gin.H{
		"balance":           user.Balance,
		"totalConsumption":  totalConsumption,
	})
}

// GetBalanceLogs 获取余额变动记录
// @Summary 获取余额变动记录
// @Description 获取当前登录用户的余额变动历史记录，支持分页
// @Tags 用户余额
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(10)
// @Param type query string false "类型筛选 recharge/consume/refund/bonus"
// @Success 200 {object} common.Response{data=object} "获取成功"
// @Failure 401 {object} common.Response "用户未登录"
// @Failure 500 {object} common.Response "获取失败"
// @Router /user/balance/logs [get]
func GetBalanceLogs(c *gin.Context) {
	userID, err := middleware.GetUserIDFromContext(c)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeUnauthorized, err.Error()))
		return
	}

	var pageInfo common.PageInfo
	if err := c.ShouldBindQuery(&pageInfo); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误"))
		return
	}
	pageInfo.Normalize(common.DefaultPageSize)

	logType := c.Query("type")

	var logs []productModel.UserBalanceLog
	var total int64

	query := global.APP_DB.Model(&productModel.UserBalanceLog{}).Where("user_id = ?", userID)
	if logType != "" {
		query = query.Where("type = ?", logType)
	}

	if err := query.Count(&total).Error; err != nil {
		global.APP_LOG.Error("获取余额记录总数失败", zap.Uint("userID", userID), zap.Error(err))
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	offset := (pageInfo.Page - 1) * pageInfo.PageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageInfo.PageSize).Find(&logs).Error; err != nil {
		global.APP_LOG.Error("获取余额记录失败", zap.Uint("userID", userID), zap.Error(err))
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccessWithPagination(c, logs, total, pageInfo.Page, pageInfo.PageSize)
}
