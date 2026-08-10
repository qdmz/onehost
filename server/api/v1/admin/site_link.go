package admin

import (
	"strconv"

	"oneclickvirt/model/common"
	productModel "oneclickvirt/model/product"
	"oneclickvirt/service/product"

	"github.com/gin-gonic/gin"
)

// GetAdminSiteLinkList 获取站点链接列表
func GetAdminSiteLinkList(c *gin.Context) {
	linkType := c.Query("linkType")
	statusStr := c.Query("status")
	var status *int
	if statusStr != "" {
		s, err := strconv.Atoi(statusStr)
		if err == nil {
			status = &s
		}
	}

	service := product.NewSiteLinkService()
	links, total, err := service.GetAdminSiteLinkList(linkType, status)
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccessWithPagination(c, links, total, 1, int(total))
}

// CreateAdminSiteLink 创建站点链接
func CreateAdminSiteLink(c *gin.Context) {
	var req productModel.CreateSiteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误"))
		return
	}

	service := product.NewSiteLinkService()
	link, err := service.CreateSiteLink(req)
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, link, "创建成功")
}

// GetAdminSiteLinkDetail 获取站点链接详情
func GetAdminSiteLinkDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "无效的ID"))
		return
	}

	service := product.NewSiteLinkService()
	link, err := service.GetSiteLinkByID(uint(id))
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, link)
}

// UpdateAdminSiteLink 更新站点链接
func UpdateAdminSiteLink(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "无效的ID"))
		return
	}

	var req productModel.UpdateSiteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误"))
		return
	}

	service := product.NewSiteLinkService()
	if err := service.UpdateSiteLink(uint(id), req); err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, nil, "更新成功")
}

// DeleteAdminSiteLink 删除站点链接
func DeleteAdminSiteLink(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "无效的ID"))
		return
	}

	service := product.NewSiteLinkService()
	if err := service.DeleteSiteLink(uint(id)); err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, nil, "删除成功")
}
