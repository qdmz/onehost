package product

import (
	"time"

	"gorm.io/gorm"
)

// 代金券状态
const (
	VoucherStatusUnused = 0 // 未使用
	VoucherStatusUsed   = 1 // 已使用
	VoucherStatusVoid   = 2 // 已作废
)

// Voucher 代金券（充值卡）
// 管理员批量生成券码，用户在钱包页输入券码兑换为账户余额。
type Voucher struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Code    string  `json:"code" gorm:"uniqueIndex;not null;size:64"` // 券码(唯一)
	Amount  float64 `json:"amount" gorm:"not null"`                   // 面额(元)
	BatchNo string  `json:"batchNo" gorm:"size:64;index"`             // 批次号(同一次生成共享)

	Status       int        `json:"status" gorm:"default:0;index"` // 0=未使用 1=已使用 2=已作废
	UsedByUserID uint       `json:"usedByUserId" gorm:"index"`     // 使用者用户ID
	UsedAt       *time.Time `json:"usedAt"`                        // 使用时间
	ExpireAt     *time.Time `json:"expireAt"`                      // 有效期(nil=永久有效)

	Remark    string `json:"remark" gorm:"size:256"` // 备注
	CreatedBy uint   `json:"createdBy"`              // 创建管理员ID

	// 非数据库字段：列表展示时回填使用者用户名
	UsedByUsername string `json:"usedByUsername" gorm:"-"`
}

// TableName 指定表名
func (Voucher) TableName() string {
	return "vouchers"
}

// IsExpired 判断代金券是否已过期
func (v *Voucher) IsExpired() bool {
	if v.ExpireAt == nil {
		return false
	}
	return v.ExpireAt.Before(time.Now())
}
