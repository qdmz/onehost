package admin

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"oneclickvirt/global"
	"oneclickvirt/middleware"
	"oneclickvirt/model/common"
	productModel "oneclickvirt/model/product"
	userModel "oneclickvirt/model/user"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 券码字符集：去掉容易混淆的 0/O/1/I/L
const voucherCodeCharset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// generateVoucherCode 生成指定长度的随机券码
func generateVoucherCode(length int) (string, error) {
	charsetLen := big.NewInt(int64(len(voucherCodeCharset)))
	var sb strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		sb.WriteByte(voucherCodeCharset[n.Int64()])
	}
	return sb.String(), nil
}

// CreateVouchers 批量生成代金券
// @Summary 批量生成代金券
// @Description 管理员按面额批量生成代金券券码
// @Tags 管理员代金券
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param data body product.CreateVoucherRequest true "生成参数"
// @Success 200 {object} common.Response "生成成功"
// @Failure 400 {object} common.Response "参数错误"
// @Router /admin/vouchers [post]
func CreateVouchers(c *gin.Context) {
	var req productModel.CreateVoucherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误: "+err.Error()))
		return
	}

	operatorID, _ := middleware.GetUserIDFromContext(c)
	batchNo := "B" + time.Now().Format("20060102150405")
	prefix := strings.ToUpper(strings.TrimSpace(req.Prefix))

	vouchers := make([]productModel.Voucher, 0, req.Count)
	// 券码去重：同一批次内先在内存去重，落库时再依赖唯一索引兜底
	seen := make(map[string]struct{}, req.Count)
	for i := 0; i < req.Count; i++ {
		var code string
		for attempt := 0; attempt < 8; attempt++ {
			body, err := generateVoucherCode(12)
			if err != nil {
				common.ResponseWithError(c, common.NewError(common.CodeInternalError, "生成券码失败"))
				return
			}
			candidate := prefix + body
			if _, dup := seen[candidate]; dup {
				continue
			}
			var count int64
			global.APP_DB.Model(&productModel.Voucher{}).Where("code = ?", candidate).Count(&count)
			if count > 0 {
				continue
			}
			code = candidate
			break
		}
		if code == "" {
			common.ResponseWithError(c, common.NewError(common.CodeInternalError, "生成券码冲突过多，请重试"))
			return
		}
		seen[code] = struct{}{}
		vouchers = append(vouchers, productModel.Voucher{
			Code:      code,
			Amount:    req.Amount,
			BatchNo:   batchNo,
			Status:    productModel.VoucherStatusUnused,
			ExpireAt:  req.ExpireAt,
			Remark:    req.Remark,
			CreatedBy: operatorID,
		})
	}

	if err := global.APP_DB.CreateInBatches(&vouchers, 100).Error; err != nil {
		global.APP_LOG.Error("批量生成代金券失败", zap.Error(err))
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	codes := make([]string, 0, len(vouchers))
	for _, v := range vouchers {
		codes = append(codes, v.Code)
	}

	common.ResponseSuccess(c, gin.H{
		"batchNo": batchNo,
		"count":   len(vouchers),
		"amount":  req.Amount,
		"codes":   codes,
	})
}

// GetVoucherList 获取代金券列表
// @Summary 获取代金券列表
// @Tags 管理员代金券
// @Produce json
// @Security BearerAuth
// @Success 200 {object} common.Response "获取成功"
// @Router /admin/vouchers [get]
func GetVoucherList(c *gin.Context) {
	var req productModel.VoucherListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误"))
		return
	}
	req.Normalize(common.DefaultPageSize)

	query := global.APP_DB.Model(&productModel.Voucher{})
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.BatchNo != "" {
		query = query.Where("batch_no = ?", req.BatchNo)
	}
	if req.Code != "" {
		query = query.Where("code LIKE ?", "%"+strings.ToUpper(req.Code)+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	var list []productModel.Voucher
	offset := (req.Page - 1) * req.PageSize
	if err := query.Order("id DESC").Offset(offset).Limit(req.PageSize).Find(&list).Error; err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}

	// 回填使用者用户名
	userIDs := make([]uint, 0)
	for _, v := range list {
		if v.UsedByUserID > 0 {
			userIDs = append(userIDs, v.UsedByUserID)
		}
	}
	if len(userIDs) > 0 {
		var users []userModel.User
		global.APP_DB.Select("id, username").Where("id IN ?", userIDs).Find(&users)
		nameMap := make(map[uint]string, len(users))
		for _, u := range users {
			nameMap[u.ID] = u.Username
		}
		for i := range list {
			if name, ok := nameMap[list[i].UsedByUserID]; ok {
				list[i].UsedByUsername = name
			}
		}
	}

	common.ResponseSuccessWithPagination(c, list, total, req.Page, req.PageSize)
}

// GetVoucherStats 代金券统计
// @Summary 代金券统计
// @Tags 管理员代金券
// @Produce json
// @Security BearerAuth
// @Success 200 {object} common.Response "获取成功"
// @Router /admin/vouchers/stats [get]
func GetVoucherStats(c *gin.Context) {
	type statRow struct {
		Status int     `json:"status"`
		Count  int64   `json:"count"`
		Amount float64 `json:"amount"`
	}
	var rows []statRow
	global.APP_DB.Model(&productModel.Voucher{}).
		Select("status, COUNT(*) as count, COALESCE(SUM(amount),0) as amount").
		Group("status").Scan(&rows)

	result := gin.H{
		"unusedCount":  int64(0),
		"unusedAmount": float64(0),
		"usedCount":    int64(0),
		"usedAmount":   float64(0),
		"voidCount":    int64(0),
		"totalCount":   int64(0),
	}
	var totalCount int64
	for _, r := range rows {
		totalCount += r.Count
		switch r.Status {
		case productModel.VoucherStatusUnused:
			result["unusedCount"] = r.Count
			result["unusedAmount"] = r.Amount
		case productModel.VoucherStatusUsed:
			result["usedCount"] = r.Count
			result["usedAmount"] = r.Amount
		case productModel.VoucherStatusVoid:
			result["voidCount"] = r.Count
		}
	}
	result["totalCount"] = totalCount

	common.ResponseSuccess(c, result)
}

// VoidVoucher 作废单张代金券
// @Summary 作废代金券
// @Tags 管理员代金券
// @Produce json
// @Security BearerAuth
// @Param id path int true "代金券ID"
// @Success 200 {object} common.Response "作废成功"
// @Router /admin/vouchers/{id}/void [put]
func VoidVoucher(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "无效的代金券ID"))
		return
	}

	var voucher productModel.Voucher
	if err := global.APP_DB.First(&voucher, uint(id)).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeNotFound, "代金券不存在"))
		return
	}
	if voucher.Status == productModel.VoucherStatusUsed {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "该代金券已被使用，无法作废"))
		return
	}

	if err := global.APP_DB.Model(&voucher).Update("status", productModel.VoucherStatusVoid).Error; err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}
	common.ResponseSuccess(c, gin.H{"message": "已作废"})
}

// DeleteVoucher 删除单张代金券
// @Summary 删除代金券
// @Tags 管理员代金券
// @Produce json
// @Security BearerAuth
// @Param id path int true "代金券ID"
// @Success 200 {object} common.Response "删除成功"
// @Router /admin/vouchers/{id} [delete]
func DeleteVoucher(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "无效的代金券ID"))
		return
	}

	var voucher productModel.Voucher
	if err := global.APP_DB.First(&voucher, uint(id)).Error; err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeNotFound, "代金券不存在"))
		return
	}
	if voucher.Status == productModel.VoucherStatusUsed {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "该代金券已被使用，不允许删除"))
		return
	}

	if err := global.APP_DB.Delete(&voucher).Error; err != nil {
		common.ResponseWithError(c, common.ClassifyError(err))
		return
	}
	common.ResponseSuccess(c, gin.H{"message": "已删除"})
}

// BatchDeleteVouchers 批量删除代金券（按ID或批次号，已使用的会自动跳过）
// @Summary 批量删除代金券
// @Tags 管理员代金券
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param data body product.VoucherBatchDeleteRequest true "删除条件"
// @Success 200 {object} common.Response "删除成功"
// @Router /admin/vouchers/batch-delete [post]
func BatchDeleteVouchers(c *gin.Context) {
	var req productModel.VoucherBatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误"))
		return
	}
	if len(req.IDs) == 0 && req.BatchNo == "" {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "请指定要删除的代金券ID或批次号"))
		return
	}

	query := global.APP_DB.Where("status <> ?", productModel.VoucherStatusUsed)
	if len(req.IDs) > 0 {
		query = query.Where("id IN ?", req.IDs)
	}
	if req.BatchNo != "" {
		query = query.Where("batch_no = ?", req.BatchNo)
	}

	result := query.Delete(&productModel.Voucher{})
	if result.Error != nil {
		common.ResponseWithError(c, common.ClassifyError(result.Error))
		return
	}

	common.ResponseSuccess(c, gin.H{
		"deleted": result.RowsAffected,
		"message": fmt.Sprintf("已删除 %d 张未使用的代金券", result.RowsAffected),
	})
}
