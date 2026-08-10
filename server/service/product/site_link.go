package product

import (
	"errors"

	"oneclickvirt/global"
	"oneclickvirt/model/common"
	productModel "oneclickvirt/model/product"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SiteLinkService 站点链接服务
type SiteLinkService struct{}

// NewSiteLinkService 创建站点链接服务实例
func NewSiteLinkService() *SiteLinkService {
	return &SiteLinkService{}
}

// GetPublicSiteLinks 获取公开的站点链接列表
func (s *SiteLinkService) GetPublicSiteLinks(linkType string) ([]productModel.SiteLink, error) {
	var links []productModel.SiteLink
	db := global.APP_DB.Where("status = ?", 1)
	if linkType != "" {
		db = db.Where("link_type = ?", linkType)
	}
	if err := db.Order("sort_order DESC, id ASC").Find(&links).Error; err != nil {
		return nil, common.NewError(common.CodeDatabaseError, err.Error())
	}
	return links, nil
}

// GetAdminSiteLinkList 管理员获取站点链接列表
func (s *SiteLinkService) GetAdminSiteLinkList(linkType string, status *int) ([]productModel.SiteLink, int64, error) {
	var links []productModel.SiteLink
	var total int64
	db := global.APP_DB.Model(&productModel.SiteLink{})
	if linkType != "" {
		db = db.Where("link_type = ?", linkType)
	}
	if status != nil {
		db = db.Where("status = ?", *status)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, common.NewError(common.CodeDatabaseError, err.Error())
	}
	if err := db.Order("sort_order DESC, id ASC").Find(&links).Error; err != nil {
		return nil, 0, common.NewError(common.CodeDatabaseError, err.Error())
	}
	return links, total, nil
}

// GetSiteLinkByID 根据ID获取站点链接
func (s *SiteLinkService) GetSiteLinkByID(id uint) (*productModel.SiteLink, error) {
	var link productModel.SiteLink
	if err := global.APP_DB.First(&link, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewError(common.CodeNotFound, "站点链接不存在")
		}
		return nil, common.NewError(common.CodeDatabaseError, err.Error())
	}
	return &link, nil
}

// CreateSiteLink 创建站点链接
func (s *SiteLinkService) CreateSiteLink(req productModel.CreateSiteLinkRequest) (*productModel.SiteLink, error) {
	link := productModel.SiteLink{
		Name:        req.Name,
		URL:         req.URL,
		IconURL:     req.IconURL,
		LinkType:    req.LinkType,
		SortOrder:   req.SortOrder,
		Status:      req.Status,
		Description: req.Description,
	}
	if err := global.APP_DB.Create(&link).Error; err != nil {
		global.APP_LOG.Error("创建站点链接失败", zap.Error(err))
		return nil, common.NewError(common.CodeDatabaseError, err.Error())
	}
	global.APP_LOG.Info("创建站点链接成功", zap.Uint("id", link.ID), zap.String("name", link.Name))
	return &link, nil
}

// UpdateSiteLink 更新站点链接
func (s *SiteLinkService) UpdateSiteLink(id uint, req productModel.UpdateSiteLinkRequest) error {
	result := global.APP_DB.Model(&productModel.SiteLink{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":        req.Name,
		"url":         req.URL,
		"icon_url":    req.IconURL,
		"link_type":   req.LinkType,
		"sort_order":  req.SortOrder,
		"status":      req.Status,
		"description": req.Description,
	})
	if result.Error != nil {
		global.APP_LOG.Error("更新站点链接失败", zap.Error(result.Error))
		return common.NewError(common.CodeDatabaseError, result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return common.NewError(common.CodeNotFound, "站点链接不存在")
	}
	global.APP_LOG.Info("更新站点链接成功", zap.Uint("id", id))
	return nil
}

// DeleteSiteLink 删除站点链接
func (s *SiteLinkService) DeleteSiteLink(id uint) error {
	result := global.APP_DB.Delete(&productModel.SiteLink{}, id)
	if result.Error != nil {
		global.APP_LOG.Error("删除站点链接失败", zap.Error(result.Error))
		return common.NewError(common.CodeDatabaseError, result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return common.NewError(common.CodeNotFound, "站点链接不存在")
	}
	global.APP_LOG.Info("删除站点链接成功", zap.Uint("id", id))
	return nil
}

// GetRecommendedProducts 获取推荐产品列表
func (s *Service) GetRecommendedProducts(limit int) ([]productModel.Product, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	var products []productModel.Product
	if err := global.APP_DB.Where("status = ? AND is_recommended = ?", 1, true).
		Order("sort_order DESC, id ASC").
		Limit(limit).
		Find(&products).Error; err != nil {
		return nil, common.NewError(common.CodeDatabaseError, err.Error())
	}
	return products, nil
}
