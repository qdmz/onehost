package admin

import (
	"oneclickvirt/global"
	"oneclickvirt/model/common"
	productModel "oneclickvirt/model/product"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GetYiPayConfig 获取易支付配置
// @Summary 获取易支付配置
// @Description 管理员获取易支付配置信息（返回第一条记录，不存在则返回默认配置）
// @Tags 管理员易支付配置
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} common.Response{data=product.YiPayConfig} "获取成功"
// @Failure 401 {object} common.Response "未授权"
// @Failure 500 {object} common.Response "获取失败"
// @Router /admin/yipay-config [get]
func GetYiPayConfig(c *gin.Context) {
	var config productModel.YiPayConfig
	result := global.APP_DB.First(&config)
	if result.Error != nil {
		// 记录不存在时返回空配置（不报错）
		if result.RowsAffected == 0 {
			common.ResponseSuccess(c, productModel.YiPayConfig{})
			return
		}
		global.APP_LOG.Error("获取易支付配置失败", zap.Error(result.Error))
		common.ResponseWithError(c, common.ClassifyError(result.Error))
		return
	}

	common.ResponseSuccess(c, config)
}

// UpdateYiPayConfig 更新易支付配置
// @Summary 更新易支付配置
// @Description 管理员创建或更新易支付配置信息（如存在则更新，不存在则新建）
// @Tags 管理员易支付配置
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body product.YiPayConfig true "易支付配置参数"
// @Success 200 {object} common.Response{data=product.YiPayConfig} "更新成功"
// @Failure 400 {object} common.Response "参数错误"
// @Failure 401 {object} common.Response "未授权"
// @Failure 500 {object} common.Response "更新失败"
// @Router /admin/yipay-config [put]
func UpdateYiPayConfig(c *gin.Context) {
	var req productModel.YiPayConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误"))
		return
	}

	var config productModel.YiPayConfig
	result := global.APP_DB.First(&config)

	if result.Error != nil && result.RowsAffected == 0 {
		// 记录不存在，创建新记录
		config = productModel.YiPayConfig{
			Enabled:    req.Enabled,
			Name:       req.Name,
			ApiURL:     req.ApiURL,
			Pid:        req.Pid,
			Key:        req.Key,
			NotifyURL:  req.NotifyURL,
			ReturnURL:  req.ReturnURL,
			PayType:    req.PayType,
			FeePercent: req.FeePercent,
			MinAmount:  req.MinAmount,
			MaxAmount:  req.MaxAmount,
		}
		if err := global.APP_DB.Create(&config).Error; err != nil {
			global.APP_LOG.Error("创建易支付配置失败", zap.Error(err))
			common.ResponseWithError(c, common.ClassifyError(err))
			return
		}
		common.ResponseSuccess(c, config, "易支付配置创建成功")
		return
	}

	if result.Error != nil {
		global.APP_LOG.Error("查询易支付配置失败", zap.Error(result.Error))
		common.ResponseWithError(c, common.ClassifyError(result.Error))
		return
	}

	// 记录存在，更新配置
	updates := map[string]interface{}{
		"enabled":      req.Enabled,
		"name":         req.Name,
		"api_url":      req.ApiURL,
		"pid":          req.Pid,
		"key":          req.Key,
		"notify_url":   req.NotifyURL,
		"return_url":   req.ReturnURL,
		"pay_type":     req.PayType,
		"fee_percent":  req.FeePercent,
		"min_amount":   req.MinAmount,
		"max_amount":   req.MaxAmount,
	}

	if err := global.APP_DB.Model(&config).Updates(updates).Error; err != nil {
		global.APP_LOG.Error("更新易支付配置失败", zap.Error(err))
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	// 重新查询返回更新后的完整记录
	if err := global.APP_DB.First(&config, config.ID).Error; err != nil {
		global.APP_LOG.Error("查询更新后的易支付配置失败", zap.Error(err))
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, config, "易支付配置更新成功")
}
