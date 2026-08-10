package product

import (
	"strconv"

	"oneclickvirt/model/common"
	productService "oneclickvirt/service/product"

	"github.com/gin-gonic/gin"
)

// GetPublicSiteLinks 获取公开站点链接列表
func GetPublicSiteLinks(c *gin.Context) {
	linkType := c.Query("linkType")

	service := productService.NewSiteLinkService()
	links, err := service.GetPublicSiteLinks(linkType)
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, links)
}

// GetPublicRecommendedProducts 获取推荐产品列表
func GetPublicRecommendedProducts(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "8")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 8
	}

	service := productService.NewService()
	products, err := service.GetRecommendedProducts(limit)
	if err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	common.ResponseSuccess(c, products)
}
