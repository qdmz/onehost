package product

import (
	"time"

	"oneclickvirt/model/common"
)

// CreateVoucherRequest 批量生成代金券请求
type CreateVoucherRequest struct {
	Amount   float64    `json:"amount" binding:"required,gt=0"`       // 面额(元)
	Count    int        `json:"count" binding:"required,min=1,max=500"` // 生成数量
	Prefix   string     `json:"prefix" binding:"max=16"`              // 券码前缀(可选)
	ExpireAt *time.Time `json:"expireAt"`                             // 有效期(可选,nil=永久)
	Remark   string     `json:"remark" binding:"max=256"`             // 备注
}

// VoucherListRequest 代金券列表查询请求
type VoucherListRequest struct {
	common.PageInfo
	Status  *int   `json:"status" form:"status"`   // 状态筛选(nil=全部)
	BatchNo string `json:"batchNo" form:"batchNo"` // 批次号筛选
	Code    string `json:"code" form:"code"`       // 券码模糊搜索
}

// VoucherBatchDeleteRequest 批量删除/作废代金券
type VoucherBatchDeleteRequest struct {
	IDs     []uint `json:"ids"`     // 按ID批量操作
	BatchNo string `json:"batchNo"` // 按批次号批量操作(与IDs二选一)
}

// RedeemVoucherRequest 用户兑换代金券请求
type RedeemVoucherRequest struct {
	Code string `json:"code" binding:"required,max=64"` // 券码
}

// AdjustUserBalanceRequest 管理员调整用户余额请求
type AdjustUserBalanceRequest struct {
	// Mode: add=在现有余额上增减(Amount 可为负)；set=直接设为指定值(Amount 必须 >= 0)
	Mode   string  `json:"mode" binding:"required,oneof=add set"`
	Amount float64 `json:"amount"`
	Remark string  `json:"remark" binding:"max=256"`
}
