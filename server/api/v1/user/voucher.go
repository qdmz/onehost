package user

import (
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
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RedeemVoucher 用户兑换代金券充值
// @Summary 兑换代金券
// @Description 用户输入代金券券码，将面额充值到账户余额
// @Tags 用户余额
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param data body product.RedeemVoucherRequest true "券码"
// @Success 200 {object} common.Response "兑换成功"
// @Failure 400 {object} common.Response "券码无效"
// @Router /user/vouchers/redeem [post]
func RedeemVoucher(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeUnauthorized, err.Error()))
		return
	}

	var req productModel.RedeemVoucherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "请输入代金券券码"))
		return
	}

	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "请输入代金券券码"))
		return
	}

	var (
		amount       float64
		balanceAfter float64
	)

	txErr := global.APP_DB.Transaction(func(tx *gorm.DB) error {
		// 行锁住代金券，防止并发重复兑换
		var voucher productModel.Voucher
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("code = ?", code).First(&voucher).Error; err != nil {
			return common.NewError(common.CodeNotFound, "代金券不存在或券码错误")
		}
		if voucher.Status == productModel.VoucherStatusUsed {
			return common.NewError(common.CodeInvalidParam, "该代金券已被使用")
		}
		if voucher.Status == productModel.VoucherStatusVoid {
			return common.NewError(common.CodeInvalidParam, "该代金券已作废")
		}
		if voucher.IsExpired() {
			return common.NewError(common.CodeInvalidParam, "该代金券已过期")
		}

		var user userModel.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&user, userID).Error; err != nil {
			return common.NewError(common.CodeNotFound, "用户不存在")
		}

		before := user.Balance
		after := before + voucher.Amount

		if err := tx.Model(&userModel.User{}).Where("id = ?", userID).
			Update("balance", after).Error; err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(&productModel.Voucher{}).Where("id = ?", voucher.ID).
			Updates(map[string]interface{}{
				"status":          productModel.VoucherStatusUsed,
				"used_by_user_id": userID,
				"used_at":         &now,
			}).Error; err != nil {
			return err
		}

		log := productModel.UserBalanceLog{
			UserID:        userID,
			Type:          "recharge",
			Amount:        voucher.Amount,
			BalanceBefore: before,
			BalanceAfter:  after,
			Remark:        "代金券兑换: " + voucher.Code,
			TradeNo:       voucher.Code,
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}

		amount = voucher.Amount
		balanceAfter = after
		return nil
	})

	if txErr != nil {
		global.APP_LOG.Warn("代金券兑换失败", zap.Uint("userID", userID), zap.String("code", code), zap.Error(txErr))
		common.ResponseWithError(c, common.ClassifyError(txErr))
		return
	}

	common.ResponseSuccess(c, gin.H{
		"amount":  amount,
		"balance": balanceAfter,
		"message": "兑换成功",
	})
}

// AdminAdjustUserBalance 管理员调整用户余额
// @Summary 管理员调整用户余额
// @Description 管理员为指定用户增加/扣减余额，或直接设定余额，并写入余额变动记录
// @Tags 管理员用户管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Param data body product.AdjustUserBalanceRequest true "调整参数"
// @Success 200 {object} common.Response "调整成功"
// @Failure 400 {object} common.Response "参数错误"
// @Router /admin/users/{id}/balance [put]
func AdminAdjustUserBalance(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		idStr = c.Param("userId")
	}
	targetID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "无效的用户ID"))
		return
	}

	var req productModel.AdjustUserBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithError(c, common.NewError(common.CodeValidationError, "参数错误: "+err.Error()))
		return
	}
	if req.Mode == "add" && req.Amount == 0 {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "调整金额不能为 0"))
		return
	}
	if req.Mode == "set" && req.Amount < 0 {
		common.ResponseWithError(c, common.NewError(common.CodeInvalidParam, "余额不能设为负数"))
		return
	}

	operatorID, _ := middleware.GetUserIDFromContext(c)

	var (
		before float64
		after  float64
		delta  float64
	)

	txErr := global.APP_DB.Transaction(func(tx *gorm.DB) error {
		var target userModel.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&target, uint(targetID)).Error; err != nil {
			return common.NewError(common.CodeNotFound, "用户不存在")
		}

		before = target.Balance
		if req.Mode == "set" {
			after = req.Amount
		} else {
			after = before + req.Amount
		}
		if after < 0 {
			return common.NewError(common.CodeInvalidParam, "调整后余额不能为负数")
		}
		delta = after - before

		if err := tx.Model(&userModel.User{}).Where("id = ?", uint(targetID)).
			Update("balance", after).Error; err != nil {
			return err
		}

		logType := "bonus"
		if delta < 0 {
			logType = "consume"
		}
		remark := req.Remark
		if remark == "" {
			remark = "管理员调整余额"
		}
		remark = remark + "（操作管理员ID: " + strconv.FormatUint(uint64(operatorID), 10) + "）"

		log := productModel.UserBalanceLog{
			UserID:        uint(targetID),
			Type:          logType,
			Amount:        delta,
			BalanceBefore: before,
			BalanceAfter:  after,
			Remark:        remark,
		}
		return tx.Create(&log).Error
	})

	if txErr != nil {
		global.APP_LOG.Error("管理员调整用户余额失败",
			zap.Uint64("targetUserID", targetID), zap.Error(txErr))
		common.ResponseWithError(c, common.ClassifyError(txErr))
		return
	}

	global.APP_LOG.Info("管理员调整用户余额",
		zap.Uint("operator", operatorID),
		zap.Uint64("targetUserID", targetID),
		zap.Float64("before", before),
		zap.Float64("after", after))

	common.ResponseSuccess(c, gin.H{
		"balanceBefore": before,
		"balanceAfter":  after,
		"amount":        delta,
		"message":       "余额调整成功",
	})
}
