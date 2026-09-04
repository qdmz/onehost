// Package idcsmart 实现与「智简魔方(idcsmart)」开放 API 的对接客户端。
//
// 设计目标：让 OneHost 作为代理商，把智简魔方当作上游产品源进行代理销售。
//   - 读取上游产品配置与价格（ListProducts）
//   - 支付后自动开通（CreateInstance）
//   - 在线管理上游虚拟机（开机/关机/重启/重装/改密/控制台/销毁）
//
// 由于不同版本的智简魔方 API 在「请求地址、鉴权方式、动作名、字段命名」上存在差异，
// 本客户端全部做成可配置项（持久化在 Provider.AuthConfig 中）：
//   - AuthType = "api_client"：使用 API ID + API Key + sign 签名（/client/api.php 风格）
//   - AuthType = "module"    ：使用 username + password 明文鉴权（/index.php?m=api&a= 风格）
// 动作名（action_map）与字段映射可由管理员在后台覆盖，默认提供一套常见命名。
package idcsmart

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"oneclickvirt/global"
)

// Config 智简魔方上游 API 配置（保存在 Provider.AuthConfig，不返回前端）
type Config struct {
	BaseURL    string            `json:"base_url"`    // 上游 API 基地址，如 https://example.com/client/api.php 或 https://example.com/index.php
	AuthType   string            `json:"auth_type"`   // api_client | module
	APIID      string            `json:"api_id"`      // API 客户端 ID（api_client 模式）
	APIKey     string            `json:"api_key"`     // API 客户端密钥（api_client 模式）
	Username   string            `json:"username"`    // 模块 API 用户名（module 模式）
	Password   string            `json:"password"`    // 模块 API 密码（module 模式）
	SignMethod string            `json:"sign_method"` // md5（默认）
	Timeout    int               `json:"timeout"`     // 请求超时（秒），默认 30
	ActionMap  map[string]string `json:"action_map"`  // 操作名覆盖，如 {"create":"cloud_host_create"}
}

// OSInfo 操作系统选项
type OSInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UpstreamProduct 归一化后的上游产品
type UpstreamProduct struct {
	Raw         map[string]interface{} `json:"raw"`
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	CPU         int                   `json:"cpu"`
	Memory      int                   `json:"memory"`    // MB
	Disk        int                   `json:"disk"`      // MB
	Bandwidth   int                   `json:"bandwidth"` // Mbps
	Traffic     int                   `json:"traffic"`   // MB
	Price       float64               `json:"price"`
	PeriodType  string                `json:"periodType"`  // hour/day/month/year
	PeriodValue int                   `json:"periodValue"` // 周期值
	OSList      []OSInfo             `json:"osList"`
}

// InstanceResult 上游实例详情（归一化）
type InstanceResult struct {
	UpstreamID string `json:"upstreamId"`
	Name       string `json:"name"`
	Status     string `json:"status"` // running / stopped / creating / unknown
	PublicIP   string `json:"publicIp"`
	PrivateIP  string `json:"privateIp"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	OS         string `json:"os"`
	CPU        int    `json:"cpu"`
	Memory     int    `json:"memory"`   // MB
	Disk       int    `json:"disk"`     // MB
	Bandwidth  int    `json:"bandwidth"` // Mbps
	Raw        map[string]interface{} `json:"raw"`
}

// ConsoleInfo 控制台/ VNC 信息
type ConsoleInfo struct {
	Type    string `json:"type"`    // vnc / novnc / web
	URL     string `json:"url"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Password string `json:"password"`
	Raw     map[string]interface{} `json:"raw"`
}

// CreateInstanceRequest 开通实例请求
type CreateInstanceRequest struct {
	ProductID string            `json:"productId"`      // 上游产品ID
	OSID      string            `json:"osId"`          // 操作系统标识
	Hostname  string            `json:"hostname"`       // 可选
	Password  string            `json:"password"`       // 可选，不传则由上游随机生成
	Quantity  int               `json:"quantity"`       // 周期数
	Extra     map[string]string `json:"extra"`          // 其它上游特定参数
}

// Client 智简魔方 API 客户端
type Client struct {
	cfg    *Config
	client *http.Client
}

// NewClient 创建客户端
func NewClient(cfg *Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	return &Client{
		cfg:    cfg,
		client: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

// defaultAction 获取动作名（优先使用 action_map 覆盖）
func (c *Client) defaultAction(op string, def string) string {
	if c.cfg.ActionMap != nil {
		if v, ok := c.cfg.ActionMap[op]; ok && v != "" {
			return v
		}
	}
	return def
}

// sign 计算签名（默认 md5(sortedKeyValue + key)）
// 不同部署可调整；此处为最常见的「参数按 key 升序拼接后追加 key 再 md5」。
func (c *Client) sign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(c.cfg.APIKey)
	sum := md5.Sum([]byte(sb.String()))
	return fmt.Sprintf("%x", sum)
}

// call 执行一次 API 调用，返回解析后的 data 字段（map 或切片由调用方断言）
func (c *Client) call(action string, params map[string]interface{}) (map[string]interface{}, error) {
	if c.cfg.BaseURL == "" {
		return nil, fmt.Errorf("智简魔方 BaseURL 未配置")
	}

	form := url.Values{}
	for k, v := range params {
		form.Set(k, fmt.Sprintf("%v", v))
	}

	switch c.cfg.AuthType {
	case "module":
		// /index.php?m=api&a=<action> 风格：username/password 作为公共参数
		form.Set("username", c.cfg.Username)
		form.Set("password", c.cfg.Password)
	default:
		// api_client 风格：id + api + sign
		form.Set("id", c.cfg.APIID)
		form.Set("api", action)
		form.Set("sign", c.sign(map[string]string{
			"id":  c.cfg.APIID,
			"api": action,
		}))
	}

	// module 风格通过 query string 传递 a=action；api_client 风格 action 已在 sign/form 中
	reqURL := c.cfg.BaseURL
	if c.cfg.AuthType == "module" {
		sep := "?"
		if strings.Contains(reqURL, "?") {
			sep = "&"
		}
		reqURL = fmt.Sprintf("%s%sa=%s", reqURL, sep, action)
	}

	resp, err := c.client.Post(reqURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("请求智简魔方失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if global.APP_LOG != nil {
		global.APP_LOG.Debug("智简魔方 API 响应",
			zap.String("action", action),
			zap.String("resp", string(body)))
	}

	var parsed struct {
		Status string                 `json:"status"`
		Code   int                    `json:"code"`
		Msg    string                 `json:"msg"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, body=%s", err, string(body))
	}

	// 兼容两种成功判定：status=="success" 或 code==0/200
	success := parsed.Status == "success" ||
		(parsed.Status == "" && (parsed.Code == 0 || parsed.Code == 200))
	if !success {
		return nil, fmt.Errorf("智简魔方返回错误: %s (code=%d)", parsed.Msg, parsed.Code)
	}

	return parsed.Data, nil
}

// TestConnection 测试连接可用性
func (c *Client) TestConnection() error {
	// 用一个轻量动作探测：优先 listProducts，失败则用 getProductList
	_, err := c.ListProducts()
	if err != nil {
		// 某些部署没有产品列表接口，退化为测试基础连通
		return fmt.Errorf("连接测试失败: %w", err)
	}
	return nil
}

// ListProducts 拉取上游产品列表（返回原始条目，由上层映射到 OneHost 产品）
func (c *Client) ListProducts() ([]map[string]interface{}, error) {
	action := c.defaultAction("listProducts", "getProductList")
	data, err := c.call(action, map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	return extractList(data, "list", "data", "product", "products")
}

// CreateInstance 开通实例
func (c *Client) CreateInstance(req CreateInstanceRequest) (*InstanceResult, error) {
	action := c.defaultAction("create", "cloudHostCreate")
	params := map[string]interface{}{
		"product_id": req.ProductID,
		"os":         req.OSID,
	}
	if req.Hostname != "" {
		params["hostname"] = req.Hostname
	}
	if req.Password != "" {
		params["password"] = req.Password
	}
	if req.Quantity > 0 {
		params["quantity"] = req.Quantity
	}
	for k, v := range req.Extra {
		params[k] = v
	}
	data, err := c.call(action, params)
	if err != nil {
		return nil, err
	}
	return parseInstanceResult(data), nil
}

// GetInstance 获取实例详情
func (c *Client) GetInstance(upstreamID string) (*InstanceResult, error) {
	action := c.defaultAction("detail", "cloudHostDetail")
	data, err := c.call(action, map[string]interface{}{"id": upstreamID})
	if err != nil {
		return nil, err
	}
	return parseInstanceResult(data), nil
}

// Start 开机
func (c *Client) Start(upstreamID string) error {
	_, err := c.call(c.defaultAction("start", "cloudHostStart"), map[string]interface{}{"id": upstreamID})
	return err
}

// Stop 关机
func (c *Client) Stop(upstreamID string) error {
	_, err := c.call(c.defaultAction("stop", "cloudHostStop"), map[string]interface{}{"id": upstreamID})
	return err
}

// Reboot 重启
func (c *Client) Reboot(upstreamID string) error {
	_, err := c.call(c.defaultAction("reboot", "cloudHostReboot"), map[string]interface{}{"id": upstreamID})
	return err
}

// Reinstall 重装系统
func (c *Client) Reinstall(upstreamID string, osID string) error {
	params := map[string]interface{}{"id": upstreamID}
	if osID != "" {
		params["os"] = osID
	}
	_, err := c.call(c.defaultAction("reinstall", "cloudHostReinstall"), params)
	return err
}

// ResetPassword 重置密码
func (c *Client) ResetPassword(upstreamID string, password string) error {
	params := map[string]interface{}{"id": upstreamID}
	if password != "" {
		params["password"] = password
	}
	_, err := c.call(c.defaultAction("resetPassword", "cloudHostResetPassword"), params)
	return err
}

// GetConsole 获取控制台/VNC 信息
func (c *Client) GetConsole(upstreamID string) (*ConsoleInfo, error) {
	data, err := c.call(c.defaultAction("console", "cloudHostVnc"), map[string]interface{}{"id": upstreamID})
	if err != nil {
		return nil, err
	}
	ci := &ConsoleInfo{Raw: data}
	ci.Type = stringField(data, "type", "vnc")
	ci.URL = stringField(data, "url", "vnc_url", "console_url")
	ci.Host = stringField(data, "host", "ip", "address")
	ci.Port = intField(data, "port", "vnc_port")
	ci.Password = stringField(data, "password", "vnc_password")
	return ci, nil
}

// Delete 销毁实例
func (c *Client) Delete(upstreamID string) error {
	_, err := c.call(c.defaultAction("delete", "cloudHostDelete"), map[string]interface{}{"id": upstreamID})
	return err
}

// ---------- 辅助解析 ----------

func extractList(data map[string]interface{}, keys ...string) ([]map[string]interface{}, error) {
	for _, k := range keys {
		if raw, ok := data[k]; ok {
			if arr, ok := raw.([]interface{}); ok {
				out := make([]map[string]interface{}, 0, len(arr))
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						out = append(out, m)
					}
				}
				return out, nil
			}
			// 兼容 data 本身就是数组
			if arr, ok := raw.([]map[string]interface{}); ok {
				return arr, nil
			}
		}
	}
	// data 本身可能是数组
	if arr, ok := data[""].([]map[string]interface{}); ok {
		return arr, nil
	}
	return []map[string]interface{}{}, nil
}

func parseInstanceResult(data map[string]interface{}) *InstanceResult {
	r := &InstanceResult{Raw: data}
	r.UpstreamID = stringField(data, "id", "upstream_id", "host_id", "instance_id")
	r.Name = stringField(data, "name", "hostname")
	r.Status = normalizeStatus(stringField(data, "status", "state"))
	r.PublicIP = stringField(data, "ip", "public_ip", "publicIp", "zhuip")
	r.PrivateIP = stringField(data, "private_ip", "intranet_ip", "internal_ip")
	r.Username = stringField(data, "username", "user")
	r.Password = stringField(data, "password", "pwd")
	r.OS = stringField(data, "os", "os_name", "image")
	r.CPU = intField(data, "cpu")
	r.Memory = intField(data, "memory", "mem", "ram")
	r.Disk = intField(data, "disk", "disk_size")
	r.Bandwidth = intField(data, "bandwidth", "bw", "net_speed")
	return r
}

func normalizeStatus(s string) string {
	switch strings.ToLower(s) {
	case "running", "on", "active", "1":
		return "running"
	case "stopped", "off", "shutdown", "0":
		return "stopped"
	case "creating", "pending", "building":
		return "creating"
	default:
		if s == "" {
			return "unknown"
		}
		return strings.ToLower(s)
	}
}

func stringField(m map[string]interface{}, keys ...string) string {
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
				var n int
				if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

// unused 避免未使用导入告警（bytes 用于后续扩展）
var _ = bytes.MinRead
