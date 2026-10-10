package admin

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
//
// AuthConfig 在创建时必填，更新时可选（nil = 保留原有 API 配置不变）。
// 因此不使用 binding:"required"，而是在 CreateUpstreamProvider 中手动校验。
type UpstreamProviderRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Region      string                 `json:"region"`
	Country     string                 `json:"country"`
	CountryCode string                 `json:"countryCode"`
	City        string                 `json:"city"`
	AllowClaim  *bool                  `json:"allowClaim"`
	AuthConfig  map[string]interface{} `json:"authConfig"` // 智简魔方 API 配置（idcsmart.Config）；更新时留空 = 保留原配置
}

// CreateUpstreamProvider 创建智简魔方上游节点（独立子系统，不走 SSH/Agent 节点流程）
func CreateUpstreamProvider(c *gin.Context) {
	var req UpstreamProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
		return
	}

	// 创建时 AuthConfig 必填
	if len(req.AuthConfig) == 0 {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "创建上游节点必须提供 API 配置(authConfig)"))
		return
	}

	cfgJSON, err := json.Marshal(req.AuthConfig)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "authConfig 序列化失败"))
		return
	}

	// 预检：确保 BaseURL 存在且格式合法
	var cfg idcsmart.Config
	if err := json.Unmarshal(cfgJSON, &cfg); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "authConfig 格式无效: "+err.Error()))
		return
	}
	if err := cfg.Validate(); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
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
//
// 当 authConfig 为 nil 或空 map 时，保留原有 API 配置不变；
// 当 authConfig 非空时，整组替换（需通过 Validate 校验）。
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
		"name":         req.Name,
		"description":  req.Description,
		"region":       req.Region,
		"country":      req.Country,
		"country_code": req.CountryCode,
		"city":         req.City,
	}
	if req.AllowClaim != nil {
		updates["allow_claim"] = *req.AllowClaim
	}

	// authConfig 非空 = 更新 API 配置；空/nil = 保留原有
	if len(req.AuthConfig) > 0 {
		cfgJSON, err := json.Marshal(req.AuthConfig)
		if err != nil {
			common.ResponseWithError(c, common.NewError(common.CodeValidationError, "authConfig 序列化失败"))
			return
		}
		// 校验新配置
		var cfg idcsmart.Config
		if err := json.Unmarshal(cfgJSON, &cfg); err != nil {
			common.ResponseWithError(c, common.NewError(common.CodeValidationError, "authConfig 格式无效: "+err.Error()))
			return
		}
		if err := cfg.Validate(); err != nil {
			common.ResponseWithError(c, common.NewError(common.CodeValidationError, err.Error()))
			return
		}
		updates["auth_config"] = string(cfgJSON)
	}

	if err := global.APP_DB.Model(&provider).Updates(updates).Error; err != nil {
		global.APP_LOG.Error("更新智简魔方上游节点失败", zap.Uint("id", uint(id)), zap.Error(err))
		common.ResponseWithError(c, common.NewError(common.CodeDatabaseError, err.Error()))
		return
	}

	// 重新查询，返回更新后的完整节点信息
	_ = global.APP_DB.First(&provider, uint(id)).Error
	common.ResponseSuccess(c, provider, "上游节点更新成功")
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

// upstreamProviderResponse 列表/详情响应结构（附带脱敏配置状态）
type upstreamProviderResponse struct {
	providerModel.Provider
	HasConfig bool   `json:"hasConfig"` // 是否已配置 API 信息
	AuthType  string `json:"authType"`  // 鉴权方式（脱敏）
	BaseURL   string `json:"baseUrl"`   // API 地址（脱敏：仅显示 scheme+host，不显示 path/query）
}

// maskBaseURL 脱敏处理：仅保留 scheme + host，隐藏具体路径
func maskBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// 提取 scheme://host 部分
	if idx := strings.Index(raw, "://"); idx > 0 {
		rest := raw[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
			return raw[:idx+3] + rest[:slashIdx]
		}
		return raw
	}
	// 无 scheme，取第一个 / 之前的部分
	if slashIdx := strings.Index(raw, "/"); slashIdx > 0 {
		return raw[:slashIdx]
	}
	return raw
}

// buildUpstreamResponse 构建带脱敏配置状态的响应
func buildUpstreamResponse(p providerModel.Provider) upstreamProviderResponse {
	resp := upstreamProviderResponse{Provider: p}
	if strings.TrimSpace(p.AuthConfig) == "" {
		return resp
	}
	var cfg idcsmart.Config
	if err := json.Unmarshal([]byte(p.AuthConfig), &cfg); err != nil {
		return resp
	}
	resp.HasConfig = cfg.BaseURL != ""
	resp.AuthType = cfg.AuthType
	resp.BaseURL = maskBaseURL(cfg.BaseURL)
	return resp
}

// ListUpstreamProviders 列出所有智简魔方上游节点（附带脱敏配置状态）
func ListUpstreamProviders(c *gin.Context) {
	var providers []providerModel.Provider
	if err := global.APP_DB.Where("type = ?", string(constant.ProviderTypeIdcsmart)).
		Order("id asc").Find(&providers).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeDatabaseError, err.Error()))
		return
	}

	// 构建脱敏响应列表
	result := make([]upstreamProviderResponse, 0, len(providers))
	for _, p := range providers {
		result = append(result, buildUpstreamResponse(p))
	}
	common.ResponseSuccess(c, result, "ok")
}

// GetUpstreamProvider 获取单个智简魔方上游节点详情（附带脱敏配置状态）
func GetUpstreamProvider(c *gin.Context) {
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

	common.ResponseSuccess(c, buildUpstreamResponse(provider), "ok")
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

	// 额外校验 URL 格式
	if err := cfg.Validate(); err != nil {
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
// 支持选择性同步：请求体中 productTypes 非空时只同步指定类型的产品
func SyncUpstreamProducts(c *gin.Context) {
	var req struct {
		ProviderID   uint     `json:"providerId" binding:"required"`
		ProductTypes []string `json:"productTypes"` // 可选：只同步这些类型的产品
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

	synced, skipped, err := upstreamService.SyncProducts(req.ProviderID, req.ProductTypes)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInternalError, err.Error()))
		return
	}
	common.ResponseSuccess(c, gin.H{"synced": synced, "skipped": skipped}, "同步完成")
}

// GetUpstreamProductTypes 获取上游产品类型列表（用于选择性同步 UI）
func GetUpstreamProductTypes(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "无效的节点ID"))
		return
	}

	types, err := upstreamService.GetUpstreamProductTypes(uint(id))
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInternalError, err.Error()))
		return
	}
	common.ResponseSuccess(c, types, "ok")
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
