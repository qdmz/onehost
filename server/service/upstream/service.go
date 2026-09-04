// Package upstream 实现「上游对接（代理销售）」业务逻辑。
//
// 目前支持智简魔方(idcsmart)上游：将上游产品同步为 OneHost 可售产品、在用户支付后自动向上游
// 开通实例、并代理管理上游虚拟机（开机/关机/重启/重装/改密/控制台/销毁）。
package upstream

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"oneclickvirt/constant"
	"oneclickvirt/global"
	idcsmart "oneclickvirt/provider/idcsmart"
	productModel "oneclickvirt/model/product"
	providerModel "oneclickvirt/model/provider"
)

// ProductConfig 保存在 Product.UpstreamConfig 中的上游产品配置
type ProductConfig struct {
	UpstreamProductID string             `json:"upstream_product_id"` // 上游产品ID
	DefaultOS         string             `json:"default_os"`          // 默认操作系统标识
	OSList            []idcsmart.OSInfo  `json:"os_list"`
	PeriodType        string             `json:"period_type"`
	PeriodValue       int                `json:"period_value"`
	Raw               map[string]interface{} `json:"raw"`
}

// loadIDCConfig 从 Provider.AuthConfig 解析智简魔方配置
func loadIDCConfig(p *providerModel.Provider) (*idcsmart.Config, error) {
	if strings.TrimSpace(p.AuthConfig) == "" {
		return nil, fmt.Errorf("节点未配置智简魔方 API 信息")
	}
	var cfg idcsmart.Config
	if err := json.Unmarshal([]byte(p.AuthConfig), &cfg); err != nil {
		return nil, fmt.Errorf("解析上游配置失败: %w", err)
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("智简魔方 BaseURL 未配置")
	}
	if cfg.SignMethod == "" {
		cfg.SignMethod = "md5"
	}
	return &cfg, nil
}

// newClient 构建智简魔方客户端
func newClient(p *providerModel.Provider) (*idcsmart.Client, error) {
	cfg, err := loadIDCConfig(p)
	if err != nil {
		return nil, err
	}
	return idcsmart.NewClient(cfg), nil
}

// TestConnection 测试上游连通性
func TestConnection(providerID uint) error {
	var p providerModel.Provider
	if err := global.APP_DB.First(&p, providerID).Error; err != nil {
		return fmt.Errorf("节点不存在: %w", err)
	}
	cli, err := newClient(&p)
	if err != nil {
		return err
	}
	return cli.TestConnection()
}

// SyncProducts 从上游拉取产品并同步为 OneHost 可售产品
// 返回同步数量与被跳过的条目数
func SyncProducts(providerID uint) (int, int, error) {
	var p providerModel.Provider
	if err := global.APP_DB.First(&p, providerID).Error; err != nil {
		return 0, 0, fmt.Errorf("节点不存在: %w", err)
	}
	cli, err := newClient(&p)
	if err != nil {
		return 0, 0, err
	}
	rawList, err := cli.ListProducts()
	if err != nil {
		return 0, 0, err
	}

	synced, skipped := 0, 0
	for _, raw := range rawList {
		up := mapUpstreamProduct(raw)
		if up.ID == "" || up.Name == "" {
			skipped++
			continue
		}
		pc := ProductConfig{
			UpstreamProductID: up.ID,
			DefaultOS:         defaultOS(up.OSList),
			OSList:            up.OSList,
			PeriodType:        orDefault(up.PeriodType, "month"),
			PeriodValue:       orInt(up.PeriodValue, 1),
			Raw:               raw,
		}
		pcJSON, _ := json.Marshal(pc)

		// 以 (Name, upstream_type) 作为去重键；名称全局唯一，重复则更新
		var exist productModel.Product
		tx := global.APP_DB.Where("name = ? AND upstream_type = ?", up.Name, constant.UpstreamTypeIDC).
			First(&exist)
		if tx.Error == nil && exist.ID > 0 {
			updates := map[string]interface{}{
				"description":    up.Description,
				"cpu":            up.CPU,
				"memory":         up.Memory,
				"disk":           up.Disk,
				"bandwidth":      up.Bandwidth,
				"traffic":        up.Traffic,
				"price":          up.Price,
				"period_type":    pc.PeriodType,
				"period_value":   pc.PeriodValue,
				"upstream_config": string(pcJSON),
				"default_provider_id": p.ID,
				"provider_ids":    strconv.Itoa(int(p.ID)),
			}
			if err := global.APP_DB.Model(&exist).Updates(updates).Error; err != nil {
				global.APP_LOG.Warn("更新上游产品失败", zap.Uint("providerID", providerID), zap.String("name", up.Name), zap.Error(err))
				skipped++
				continue
			}
			synced++
			continue
		}

		product := productModel.Product{
			Name:              up.Name,
			Description:       up.Description,
			Type:              "vm",
			Category:          "vm",
			CPU:               up.CPU,
			Memory:            up.Memory,
			Disk:              up.Disk,
			Bandwidth:         up.Bandwidth,
			Traffic:           up.Traffic,
			Price:             up.Price,
			PeriodType:        pc.PeriodType,
			PeriodValue:       pc.PeriodValue,
			Stock:             -1, // 上游库存由上游管控，本地不限
			MaxPerUser:        0,
			Status:            1, // 默认上架即可销售（管理员可手动下架）
			UpstreamType:      constant.UpstreamTypeIDC,
			UpstreamConfig:    string(pcJSON),
			DefaultProviderID: p.ID,
			ProviderIDs:       strconv.Itoa(int(p.ID)),
		}
		if err := global.APP_DB.Create(&product).Error; err != nil {
			global.APP_LOG.Warn("创建上游产品失败", zap.Uint("providerID", providerID), zap.String("name", up.Name), zap.Error(err))
			skipped++
			continue
		}
		synced++
	}
	return synced, skipped, nil
}

// ProvisionOrder 为已支付的订单向上游开通实例（idcsmart 专用，不走本地虚拟化任务链）
func ProvisionOrder(order *productModel.ProductOrder) error {
	var product productModel.Product
	if err := global.APP_DB.First(&product, order.ProductID).Error; err != nil {
		return fmt.Errorf("产品不存在: %w", err)
	}
	if product.UpstreamType != constant.UpstreamTypeIDC {
		return fmt.Errorf("非上游产品，不应走上游开通流程")
	}
	var provider providerModel.Provider
	if err := global.APP_DB.First(&provider, product.DefaultProviderID).Error; err != nil {
		return fmt.Errorf("上游节点不存在: %w", err)
	}
	cli, err := newClient(&provider)
	if err != nil {
		return err
	}

	pc, err := parseProductConfig(product.UpstreamConfig)
	if err != nil {
		return err
	}
	osID := order.UpstreamOS
	if osID == "" {
		osID = pc.DefaultOS
	}
	if osID == "" && len(pc.OSList) > 0 {
		osID = pc.OSList[0].ID
	}
	if osID == "" {
		return fmt.Errorf("上游产品未配置操作系统，无法开通")
	}

	// 先创建本地实例记录（状态 creating），再异步轮询上游状态
	inst := providerModel.Instance{
		Name:          fmt.Sprintf("idc-%s", uuid.New().String()[:8]),
		Provider:      provider.Name,
		ProviderID:    provider.ID,
		ProviderVMID:  "pending",
		UpstreamType:  constant.UpstreamTypeIDC,
		Status:        "creating",
		InstanceType:  "vm",
		CPU:           order.CPU,
		Memory:        int64(order.Memory),
		Disk:          int64(order.Disk),
		Bandwidth:     order.Bandwidth,
		NetworkType:   "dedicated_ipv4",
		UserID:        order.UserID,
		Image:         osID,
		OSType:        osID,
	}
	if order.ExpireAt != nil {
		inst.ExpiresAt = order.ExpireAt
	}
	if err := global.APP_DB.Create(&inst).Error; err != nil {
		return fmt.Errorf("创建本地实例记录失败: %w", err)
	}
	// 回填订单关联实例（状态仍为开通中，待上游受理后再置已开通）
	_ = global.APP_DB.Model(order).Updates(map[string]interface{}{
		"instance_id": inst.ID,
		"image_id":    0,
		"image_name":  osID,
	}).Error

	// 调用上游开通
	res, err := cli.CreateInstance(idcsmart.CreateInstanceRequest{
		ProductID: pc.UpstreamProductID,
		OSID:      osID,
		Quantity:  order.Quantity,
	})
	if err != nil {
		_ = global.APP_DB.Model(&inst).Update("status", "failed").Error
		_ = global.APP_DB.Model(order).Update("provision_status", 3).Error
		return fmt.Errorf("上游开通失败: %w", err)
	}

	// 上游受理成功，标记订单已开通
	now := time.Now()
	_ = global.APP_DB.Model(order).Updates(map[string]interface{}{
		"provision_status": 2,
		"provisioned_at":   &now,
	}).Error
	// 刷新实例信息
	updates := map[string]interface{}{}
	if res.UpstreamID != "" {
		updates["provider_vm_id"] = res.UpstreamID
	}
	if res.PublicIP != "" {
		updates["public_ip"] = res.PublicIP
	}
	if res.PrivateIP != "" {
		updates["private_ip"] = res.PrivateIP
	}
	if res.Username != "" {
		updates["username"] = res.Username
	}
	if res.Password != "" {
		updates["password"] = res.Password
	}
	if res.Status != "" {
		updates["status"] = res.Status
	}
	if len(updates) > 0 {
		_ = global.APP_DB.Model(&inst).Updates(updates).Error
	}

	// 异步轮询上游，待实例 running 后补齐 IP/密码
	go pollUpstreamInstance(cli, provider.ID, inst.ID, res.UpstreamID)
	return nil
}

// pollUpstreamInstance 轮询上游实例状态，直到 running 或超时（最多约 30 分钟）
func pollUpstreamInstance(cli *idcsmart.Client, providerID, instanceID uint, upstreamID string) {
	if upstreamID == "" {
		return
	}
	for i := 0; i < 180; i++ {
		time.Sleep(10 * time.Second)
		res, err := cli.GetInstance(upstreamID)
		if err != nil {
			continue
		}
		updates := map[string]interface{}{}
		if res.Status != "" {
			updates["status"] = res.Status
		}
		if res.PublicIP != "" {
			updates["public_ip"] = res.PublicIP
		}
		if res.PrivateIP != "" {
			updates["private_ip"] = res.PrivateIP
		}
		if res.Username != "" {
			updates["username"] = res.Username
		}
		if res.Password != "" {
			updates["password"] = res.Password
		}
		if res.OS != "" {
			updates["os_type"] = res.OS
		}
		if len(updates) > 0 {
			_ = global.APP_DB.Model(&providerModel.Instance{}).Where("id = ?", instanceID).Updates(updates).Error
		}
		if res.Status == "running" {
			return
		}
	}
}

// ManageInstance 代理用户对上游实例的管理动作
// action: start/stop/reboot/reinstall/reset-password/console/delete
func ManageInstance(instanceID uint, action string, params map[string]string) (interface{}, error) {
	var inst providerModel.Instance
	if err := global.APP_DB.First(&inst, instanceID).Error; err != nil {
		return nil, fmt.Errorf("实例不存在: %w", err)
	}
	if inst.UpstreamType != constant.UpstreamTypeIDC {
		return nil, fmt.Errorf("非上游实例")
	}
	if inst.ProviderVMID == "" || inst.ProviderVMID == "pending" {
		return nil, fmt.Errorf("实例尚未关联上游 ID")
	}
	var provider providerModel.Provider
	if err := global.APP_DB.First(&provider, inst.ProviderID).Error; err != nil {
		return nil, fmt.Errorf("上游节点不存在: %w", err)
	}
	cli, err := newClient(&provider)
	if err != nil {
		return nil, err
	}

	switch action {
	case "start":
		if err := cli.Start(inst.ProviderVMID); err != nil {
			return nil, err
		}
		_ = global.APP_DB.Model(&inst).Update("status", "running").Error
		return ginH("started"), nil
	case "stop":
		if err := cli.Stop(inst.ProviderVMID); err != nil {
			return nil, err
		}
		_ = global.APP_DB.Model(&inst).Update("status", "stopped").Error
		return ginH("stopped"), nil
	case "reboot":
		if err := cli.Reboot(inst.ProviderVMID); err != nil {
			return nil, err
		}
		return ginH("rebooted"), nil
	case "reinstall":
		osID := ""
		if params != nil {
			osID = params["osId"]
		}
		if err := cli.Reinstall(inst.ProviderVMID, osID); err != nil {
			return nil, err
		}
		return ginH("reinstalling"), nil
	case "reset-password":
		pwd := ""
		if params != nil {
			pwd = params["password"]
		}
		if err := cli.ResetPassword(inst.ProviderVMID, pwd); err != nil {
			return nil, err
		}
		return ginH("password_reset"), nil
	case "console":
		ci, err := cli.GetConsole(inst.ProviderVMID)
		if err != nil {
			return nil, err
		}
		return ci, nil
	case "delete":
		if err := cli.Delete(inst.ProviderVMID); err != nil {
			return nil, err
		}
		_ = global.APP_DB.Delete(&inst).Error
		return ginH("deleted"), nil
	default:
		return nil, fmt.Errorf("不支持的动作: %s", action)
	}
}

// DefaultUpstreamOS 从 Product.UpstreamConfig 解析默认操作系统标识；
// 优先取 DefaultOS，缺失则回退 OSList[0].ID，再无则返回空。
func DefaultUpstreamOS(raw string) string {
	pc, err := parseProductConfig(raw)
	if err != nil || pc == nil {
		return ""
	}
	if pc.DefaultOS != "" {
		return pc.DefaultOS
	}
	if len(pc.OSList) > 0 {
		return pc.OSList[0].ID
	}
	return ""
}

// ---------- 工具函数 ----------

func parseProductConfig(raw string) (*ProductConfig, error) {
	var pc ProductConfig
	if err := json.Unmarshal([]byte(raw), &pc); err != nil {
		return nil, fmt.Errorf("解析上游产品配置失败: %w", err)
	}
	return &pc, nil
}

func mapUpstreamProduct(raw map[string]interface{}) idcsmart.UpstreamProduct {
	up := idcsmart.UpstreamProduct{Raw: raw}
	up.ID = strField(raw, "id", "product_id", "pid", "upstream_id")
	up.Name = strField(raw, "name", "product_name", "title")
	up.Description = strField(raw, "description", "desc", "remark")
	up.CPU = intField(raw, "cpu", "cpu_core", "cores")
	up.Memory = intField(raw, "memory", "mem", "ram", "memory_mb")
	up.Disk = intField(raw, "disk", "disk_size", "disk_mb", "storage")
	up.Bandwidth = intField(raw, "bandwidth", "bw", "net_speed", "bandwidth_mbps")
	up.Traffic = intField(raw, "traffic", "flow", "traffic_mb")
	up.Price = floatField(raw, "price", "amount", "fee")
	up.PeriodType = strField(raw, "period_type", "billing_cycle", "cycle")
	up.PeriodValue = intField(raw, "period_value", "cycle_value", "billing_value")
	if oss, ok := raw["os_list"].([]interface{}); ok {
		for _, o := range oss {
			if m, ok := o.(map[string]interface{}); ok {
				up.OSList = append(up.OSList, idcsmart.OSInfo{
					ID:   strField(m, "id", "os_id", "value"),
					Name: strField(m, "name", "os_name", "label"),
				})
			}
		}
	}
	// 单位兼容：上游若以 GB 计，转换为 MB
	if up.Memory > 0 && up.Memory <= 1024*1024 && looksLikeGB(raw, "memory", "mem", "ram") {
		up.Memory *= 1024
	}
	if up.Disk > 0 && up.Disk <= 1024*1024 && looksLikeGB(raw, "disk", "disk_size", "storage") {
		up.Disk *= 1024
	}
	return up
}

func looksLikeGB(raw map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		if _, ok := raw[k+"_gb"]; ok {
			return true
		}
	}
	return false
}

func defaultOS(list []idcsmart.OSInfo) string {
	if len(list) > 0 {
		return list[0].ID
	}
	return ""
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func orInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func strField(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func intField(m map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				return int(t)
			case int:
				return t
			case string:
				if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func floatField(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				return t
			case int:
				return float64(t)
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

// ginH 占位返回
func ginH(msg string) map[string]string {
	return map[string]string{"status": msg}
}
