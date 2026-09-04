package admin

import (
	"encoding/json"
	"fmt"
	"strconv"

	"oneclickvirt/constant"
	"oneclickvirt/global"
	"oneclickvirt/middleware"
	"oneclickvirt/model/common"
	providerModel "oneclickvirt/model/provider"
	idcsmart "oneclickvirt/provider/idcsmart"
	upstreamService "oneclickvirt/service/upstream"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UpstreamProviderRequest 智简魔方上游节点创建/更新请求
type UpstreamProviderRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Region      string                 `json:"region"`
	Country     string                 `json:"country"`
	CountryCode string                 `json:"countryCode"`
	City        string                 `json:"city"`
	AllowClaim  *bool                  `json:"allowClaim"`
	AuthConfig  map[string]interface{} `json:"authConfig" binding:"required"` // 智简魔方 API 配置（idcsmart.Config）
}

// CreateUpstreamProvider 创建智简魔方上游节点（独立子系统，不走 SSH/Agent 节点流程）
func CreateUpstreamProvider(c *gin.Context) {
	var req UpstreamProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
		return
	}

	cfgJSON, err := json.Marshal(req.AuthConfig)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "authConfig 序列化失败"))
		return
	}

	allowClaim := true
	if req.AllowClaim != nil {
		allowClaim = *req.AllowClaim
	}

	provider := providerModel.Provider{
		Name:           req.Name,
		Description:    req.Description,
		Type:           string(constant.ProviderTypeIdcsmart),
		ConnectionType: constant.UpstreamConnectionType,
		AuthConfig:     string(cfgJSON),
		Status:         "active",
		Region:         req.Region,
		Country:        req.Country,
		CountryCode:    req.CountryCode,
		City:           req.City,
		AllowClaim:     allowClaim,
		UUID:           uuid.New().String(),
	}

	if err := global.APP_DB.Create(&provider).Error; err != nil {
		global.APP_LOG.Error("创建智简魔方上游节点失败", zap.String("name", req.Name), zap.Error(err))
		common.ResponseWithError(c, common.NewError(common.CodeDatabaseError, err.Error()))
		return
	}

	common.ResponseSuccess(c, provider, "上游节点创建成功")
}

// UpdateUpstreamProvider 更新智简魔方上游节点
func UpdateUpstreamProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "无效的节点ID"))
		return
	}

	var req UpstreamProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
		return
	}

	var provider providerModel.Provider
	if err := global.APP_DB.First(&provider, uint(id)).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeNotFound, "上游节点不存在"))
		return
	}
	if provider.Type != string(constant.ProviderTypeIdcsmart) {
		common.ResponseWithError(c, common.NewError(common.CodeBadRequest, "该节点不是智简魔方上游节点"))
		return
	}

	updates := map[string]interface{}{
		"name":        req.Name,
		"description": req.Description,
		"region":      req.Region,
		"country":     req.Country,
		"country_code": req.CountryCode,
		"city":        req.City,
	}
	if req.AllowClaim != nil {
		updates["allow_claim"] = *req.AllowClaim
	}
	if req.AuthConfig != nil {
		cfgJSON, err := json.Marshal(req.AuthConfig)
		if err != nil {
			common.ResponseWithError(c, common.NewError(common.CodeValidationError, "authConfig 序列化失败"))
			return
		}
		updates["auth_config"] = string(cfgJSON)
	}

	if err := global.APP_DB.Model(&provider).Updates(updates).Error; err != nil {
		global.APP_LOG.Error("更新智简魔方上游节点失败", zap.Uint("id", uint(id)), zap.Error(err))
		common.ResponseWithError(c, common.NewError(common.CodeDatabaseError, err.Error()))
		return
	}

	common.ResponseSuccess(c, nil, "上游节点更新成功")
}

// DeleteUpstreamProvider 删除智简魔方上游节点
func DeleteUpstreamProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "无效的节点ID"))
		return
	}

	var provider providerModel.Provider
	if err := global.APP_DB.First(&provider, uint(id)).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeNotFound, "上游节点不存在"))
		return
	}
	if provider.Type != string(constant.ProviderTypeIdcsmart) {
		common.ResponseWithError(c, common.NewError(common.CodeBadRequest, "该节点不是智简魔方上游节点"))
		return
	}

	// 软删除节点；已同步的产品与运行中实例保留，仅停止向该上游开新单
	if err := global.APP_DB.Delete(&provider).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeDatabaseError, err.Error()))
		return
	}

	common.ResponseSuccess(c, nil, "上游节点已删除")
}

// ListUpstreamProviders 列出所有智简魔方上游节点
func ListUpstreamProviders(c *gin.Context) {
	var providers []providerModel.Provider
	if err := global.APP_DB.Where("type = ?", string(constant.ProviderTypeIdcsmart)).
		Order("id asc").Find(&providers).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeDatabaseError, err.Error()))
		return
	}
	// AuthConfig 已被模型标记为 json:"-"，不会返回给前端
	common.ResponseSuccess(c, providers, "ok")
}

// TestUpstreamConnection 测试智简魔方 API 连通性
// 支持两种用法：
//  1. 传入 providerId：测试已保存的节点；
//  2. 传入 authConfig：测试尚未保存的配置（保存前预检）。
func TestUpstreamConnection(c *gin.Context) {
	var req struct {
		ProviderID uint                   `json:"providerId"`
		AuthConfig map[string]interface{} `json:"authConfig"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
		return
	}

	cfg, err := resolveIDCConfig(req.ProviderID, req.AuthConfig)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeBadRequest, err.Error()))
		return
	}

	cli := idcsmart.NewClient(cfg)
	if err := cli.TestConnection(); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeBadRequest, "连接测试失败: "+err.Error()))
		return
	}
	common.ResponseSuccess(c, gin.H{"status": "ok"}, "连接成功")
}

// SyncUpstreamProducts 从上游同步产品为可售产品
func SyncUpstreamProducts(c *gin.Context) {
	var req struct {
		ProviderID uint `json:"providerId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
		return
	}

	ownerAdminID := middleware.GetOwnerAdminID(c)
	if ownerAdminID > 0 {
		var p providerModel.Provider
		if err := global.APP_DB.First(&p, req.ProviderID).Error; err == nil {
			if p.OwnerAdminID != ownerAdminID {
				common.ResponseWithError(c, common.NewError(common.CodeForbidden, "无权操作该上游节点"))
				return
			}
		}
	}

	synced, skipped, err := upstreamService.SyncProducts(req.ProviderID)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInternalError, err.Error()))
		return
	}
	common.ResponseSuccess(c, gin.H{"synced": synced, "skipped": skipped}, "同步完成")
}

// resolveIDCConfig 从已保存节点或内联配置构造智简魔方客户端配置
func resolveIDCConfig(providerID uint, inline map[string]interface{}) (*idcsmart.Config, error) {
	if providerID > 0 {
		var p providerModel.Provider
		if err := global.APP_DB.First(&p, providerID).Error; err != nil {
			return nil, fmt.Errorf("节点不存在: %w", err)
		}
		var cfg idcsmart.Config
		if err := json.Unmarshal([]byte(p.AuthConfig), &cfg); err != nil {
			return nil, fmt.Errorf("解析节点配置失败: %w", err)
		}
		return &cfg, nil
	}
	if len(inline) == 0 {
		return nil, fmt.Errorf("请提供 providerId 或 authConfig")
	}
	raw, err := json.Marshal(inline)
	if err != nil {
		return nil, fmt.Errorf("配置序列化失败: %w", err)
	}
	var cfg idcsmart.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	return &cfg, nil
}
