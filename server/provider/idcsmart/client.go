// Package idcsmart 实现与「智简魔方(idcsmart)」开放 API 的对接客户端。
//
// 支持三种鉴权方式：
//   - AuthType = "v10"       ：V10 RESTful API（/v1/ 路径），session cookie 鉴权
//   - AuthType = "api_client"：旧版 API ID + API Key + sign 签名（/client/api.php 风格）
//   - AuthType = "module"    ：旧版 username + password 明文鉴权（/index.php?m=api&a= 风格）
//
// V10 API 要点：
//   - 产品列表 GET /v1/products 是公开接口，无需登录即可获取
//   - 产品详情 GET /v1/products/{id} 需要登录 session
//   - 产品列表结构: data.first_group[].group[].products[]，每个 product 有 type 字段
//   - 产品类型: dcimcloud(云服务器), dcim(独立服务器), other(其他), server(物理机) 等
package idcsmart

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"oneclickvirt/global"
)

// Config 智简魔方上游 API 配置
type Config struct {
	BaseURL      string            `json:"base_url"`      // 上游站点基地址
	AuthType     string            `json:"auth_type"`     // v10 | api_client | module
	APIID        string            `json:"api_id"`        // API 客户端 ID（api_client 模式）
	APIKey       string            `json:"api_key"`       // API 客户端密钥（api_client 模式）
	Username     string            `json:"username"`      // 用户名（module 模式 / V10 登录备用）
	Password     string            `json:"password"`      // 密码（module 模式 / V10 登录密码）
	Email        string            `json:"email"`         // V10 登录邮箱
	SignMethod   string            `json:"sign_method"`   // md5（默认）
	Timeout      int               `json:"timeout"`       // 请求超时（秒），默认 30
	ActionMap    map[string]string `json:"action_map"`    // 操作名覆盖（旧版 API 用）
	ProductTypes []string          `json:"product_types"` // 选择性同步：只同步这些类型
}

// Validate 校验配置完整性
func (c *Config) Validate() error {
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	if c.BaseURL == "" {
		return fmt.Errorf("API 地址(BaseURL)未设置，请在上游节点配置中填写智简魔方 API 地址")
	}
	if !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
		return fmt.Errorf("API 地址必须以 http:// 或 https:// 开头，当前值: %s", c.BaseURL)
	}
	parsed, err := url.Parse(c.BaseURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("API 地址格式无效: %s", c.BaseURL)
	}
	switch c.AuthType {
	case "v10":
		// V10 产品列表是公开接口，仅 BaseURL 即可工作
	case "module":
		if c.Username == "" || c.Password == "" {
			return fmt.Errorf("module 鉴权方式需要填写用户名和密码")
		}
	default:
		if c.APIID == "" || c.APIKey == "" {
			return fmt.Errorf("api_client 鉴权方式需要填写 API ID 和 API Key")
		}
	}
	return nil
}

// OSInfo 操作系统选项
type OSInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UpstreamProduct 归一化后的上游产品
type UpstreamProduct struct {
	Raw         map[string]interface{} `json:"raw"`
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	CPU         int                    `json:"cpu"`
	Memory      int                    `json:"memory"`
	Disk        int                    `json:"disk"`
	Bandwidth   int                    `json:"bandwidth"`
	Traffic     int                    `json:"traffic"`
	Price       float64                `json:"price"`
	PeriodType  string                 `json:"periodType"`
	PeriodValue int                    `json:"periodValue"`
	OSList      []OSInfo               `json:"osList"`
}

// InstanceResult 上游实例详情
type InstanceResult struct {
	UpstreamID string                 `json:"upstreamId"`
	Name       string                 `json:"name"`
	Status     string                 `json:"status"`
	PublicIP   string                 `json:"publicIp"`
	PrivateIP  string                 `json:"privateIp"`
	Username   string                 `json:"username"`
	Password   string                 `json:"password"`
	OS         string                 `json:"os"`
	CPU        int                    `json:"cpu"`
	Memory     int                    `json:"memory"`
	Disk       int                    `json:"disk"`
	Bandwidth  int                    `json:"bandwidth"`
	Raw        map[string]interface{} `json:"raw"`
}

// ConsoleInfo 控制台/VNC 信息
type ConsoleInfo struct {
	Type     string                 `json:"type"`
	URL      string                 `json:"url"`
	Host     string                 `json:"host"`
	Port     int                    `json:"port"`
	Password string                 `json:"password"`
	Raw      map[string]interface{} `json:"raw"`
}

// CreateInstanceRequest 开通实例请求
type CreateInstanceRequest struct {
	ProductID string            `json:"productId"`
	OSID      string            `json:"osId"`
	Hostname  string            `json:"hostname"`
	Password  string            `json:"password"`
	Quantity  int               `json:"quantity"`
	Extra     map[string]string `json:"extra"`
}

// ProductTypeInfo 上游产品类型信息（用于选择性同步 UI）
type ProductTypeInfo struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Count int    `json:"count"`
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
	jar, _ := cookiejar.New(nil)
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
			Jar:     jar,
		},
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

// sign 计算签名（旧版 api_client 模式用）
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

// v10BaseURL 返回 V10 API 的基地址（保留 scheme://host，去除多余路径）
func (c *Client) v10BaseURL() string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if idx := strings.Index(base, "://"); idx > 0 {
		rest := base[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
			base = base[:idx+3] + rest[:slashIdx]
		}
	}
	return base
}

// v10Request 执行 V10 API 请求
func (c *Client) v10Request(method, path string, body interface{}) ([]byte, int, error) {
	base := c.v10BaseURL()
	reqURL := base + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("序列化请求体失败: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("请求 V10 API 失败(地址: %s): %w", maskURL(reqURL), err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读取响应失败: %w", err)
	}

	if global.APP_LOG != nil {
		global.APP_LOG.Debug("V10 API 响应",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", resp.StatusCode),
			zap.Int("bodyLen", len(respBody)))
	}

	return respBody, resp.StatusCode, nil
}

// v10Login 登录 V10 API 获取 session（如果站点启用了验证码可能会失败）
func (c *Client) v10Login() error {
	email := c.cfg.Email
	if email == "" {
		email = c.cfg.Username
	}
	if email == "" {
		return fmt.Errorf("V10 登录需要配置邮箱(email)")
	}
	if c.cfg.Password == "" {
		return fmt.Errorf("V10 登录需要配置密码(password)")
	}

	body := map[string]string{
		"type":     "password",
		"account":  email,
		"password": c.cfg.Password,
	}
	respBody, status, err := c.v10Request("POST", "/v1/login", body)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("V10 登录失败(HTTP %d): %s", status, truncStr(string(respBody), 200))
	}

	var parsed struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return fmt.Errorf("解析登录响应失败: %w", err)
	}
	if parsed.Status != 200 {
		return fmt.Errorf("V10 登录失败: %s", parsed.Msg)
	}
	return nil
}

// call 执行一次旧版 API 调用（api_client / module 模式）
func (c *Client) call(action string, params map[string]interface{}) (map[string]interface{}, error) {
	if c.cfg.BaseURL == "" {
		return nil, fmt.Errorf("API 地址(BaseURL)未设置")
	}
	if !strings.HasPrefix(c.cfg.BaseURL, "http://") && !strings.HasPrefix(c.cfg.BaseURL, "https://") {
		return nil, fmt.Errorf("API 地址格式无效(非 http/https): %s", c.cfg.BaseURL)
	}

	form := url.Values{}
	for k, v := range params {
		form.Set(k, fmt.Sprintf("%v", v))
	}

	switch c.cfg.AuthType {
	case "module":
		form.Set("username", c.cfg.Username)
		form.Set("password", c.cfg.Password)
	default:
		form.Set("id", c.cfg.APIID)
		form.Set("api", action)
		form.Set("sign", c.sign(map[string]string{
			"id":  c.cfg.APIID,
			"api": action,
		}))
	}

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
		return nil, fmt.Errorf("请求智简魔方 API 失败(地址: %s): %w", maskURL(reqURL), err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if global.APP_LOG != nil {
		global.APP_LOG.Debug("智简魔方 API 响应",
			zap.String("action", action),
			zap.String("resp", string(respBody)))
	}

	var parsed struct {
		Status string                 `json:"status"`
		Code   int                    `json:"code"`
		Msg    string                 `json:"msg"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, body=%s", err, string(respBody))
	}

	success := parsed.Status == "success" ||
		(parsed.Status == "" && (parsed.Code == 0 || parsed.Code == 200))
	if !success {
		return nil, fmt.Errorf("智简魔方返回错误: %s (code=%d)", parsed.Msg, parsed.Code)
	}

	return parsed.Data, nil
}

// TestConnection 测试连接可用性
func (c *Client) TestConnection() error {
	_, err := c.ListProducts()
	if err != nil {
		return fmt.Errorf("连接测试失败: %w", err)
	}
	return nil
}

// ListProducts 拉取上游产品列表
// V10 模式: GET /v1/products（公开接口，无需登录）
// 旧版模式: 调用 getProductList action
func (c *Client) ListProducts() ([]map[string]interface{}, error) {
	if c.cfg.AuthType == "v10" {
		return c.v10ListProducts()
	}
	action := c.defaultAction("listProducts", "getProductList")
	data, err := c.call(action, map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	return extractList(data, "list", "data", "product", "products"), nil
}

// v10ListProducts 从 V10 API 获取产品列表并展平嵌套结构
// V10 结构: data.first_group[].group[].products[]
func (c *Client) v10ListProducts() ([]map[string]interface{}, error) {
	respBody, status, err := c.v10Request("GET", "/v1/products", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("V10 产品列表请求失败(HTTP %d)", status)
	}

	var parsed struct {
		Status int                    `json:"status"`
		Msg    string                 `json:"msg"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("解析 V10 产品列表失败: %w", err)
	}
	if parsed.Status != 200 {
		return nil, fmt.Errorf("V10 API 返回错误: %s", parsed.Msg)
	}

	var products []map[string]interface{}

	// 展平 data.first_group[].group[].products[]
	firstGroups, _ := parsed.Data["first_group"].([]interface{})
	for _, fg := range firstGroups {
		fgMap, ok := fg.(map[string]interface{})
		if !ok {
			continue
		}
		groups, _ := fgMap["group"].([]interface{})
		for _, g := range groups {
			gMap, ok := g.(map[string]interface{})
			if !ok {
				continue
			}
			prods, _ := gMap["products"].([]interface{})
			for _, p := range prods {
				if pm, ok := p.(map[string]interface{}); ok {
					products = append(products, pm)
				}
			}
		}
	}

	return products, nil
}

// GetProductTypes 获取上游产品类型列表及各类型产品数量（用于选择性同步 UI）
func (c *Client) GetProductTypes() ([]ProductTypeInfo, error) {
	products, err := c.ListProducts()
	if err != nil {
		return nil, err
	}

	typeCount := map[string]int{}
	for _, p := range products {
		ptype := stringField(p, "type")
		if ptype == "" {
			ptype = "other"
		}
		typeCount[ptype]++
	}

	// 按数量降序排列
	result := make([]ProductTypeInfo, 0, len(typeCount))
	for t, count := range typeCount {
		result = append(result, ProductTypeInfo{
			Type:  t,
			Name:  productTypeName(t),
			Count: count,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result, nil
}

// productTypeName 将上游产品类型标识映射为中文名称
func productTypeName(t string) string {
	switch t {
	case "dcimcloud":
		return "云服务器"
	case "dcim":
		return "独立服务器"
	case "server":
		return "物理机"
	case "other":
		return "其他"
	case "host":
		return "虚拟主机"
	case "cdn":
		return "CDN"
	case "scdn":
		return "SCDN/云盾"
	default:
		return t
	}
}

// ProductTypeToCategory 将上游产品类型映射为 OneHost 产品类别
func ProductTypeToCategory(t string) string {
	switch t {
	case "dcimcloud":
		return "cloud_server"
	case "dcim":
		return "dedicated_server"
	case "server":
		return "dedicated_server"
	case "host":
		return "virtual_host"
	case "cdn", "scdn":
		return "cdn"
	default:
		return "other"
	}
}

// CreateInstance 开通实例
func (c *Client) CreateInstance(req CreateInstanceRequest) (*InstanceResult, error) {
	if c.cfg.AuthType == "v10" {
		return c.v10CreateInstance(req)
	}
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

// v10CreateInstance V10 API 开通实例（需要登录 session）
func (c *Client) v10CreateInstance(req CreateInstanceRequest) (*InstanceResult, error) {
	// 确保已登录
	if err := c.v10Login(); err != nil {
		return nil, fmt.Errorf("V10 登录失败(下单前需要登录): %w", err)
	}

	body := map[string]interface{}{
		"product_id": req.ProductID,
	}
	if req.OSID != "" {
		body["os"] = req.OSID
	}
	if req.Hostname != "" {
		body["hostname"] = req.Hostname
	}
	if req.Password != "" {
		body["password"] = req.Password
	}
	if req.Quantity > 0 {
		body["quantity"] = req.Quantity
	}
	for k, v := range req.Extra {
		body[k] = v
	}

	respBody, status, err := c.v10Request("POST", "/v1/order", body)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("V10 下单失败(HTTP %d): %s", status, truncStr(string(respBody), 300))
	}

	var parsed struct {
		Status int                    `json:"status"`
		Msg    string                 `json:"msg"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("解析下单响应失败: %w", err)
	}
	if parsed.Status != 200 {
		return nil, fmt.Errorf("V10 下单失败: %s", parsed.Msg)
	}
	return parseInstanceResult(parsed.Data), nil
}

// GetInstance 获取实例详情
func (c *Client) GetInstance(upstreamID string) (*InstanceResult, error) {
	if c.cfg.AuthType == "v10" {
		return c.v10GetInstance(upstreamID)
	}
	action := c.defaultAction("detail", "cloudHostDetail")
	data, err := c.call(action, map[string]interface{}{"id": upstreamID})
	if err != nil {
		return nil, err
	}
	return parseInstanceResult(data), nil
}

// v10GetInstance V10 API 获取实例详情
func (c *Client) v10GetInstance(upstreamID string) (*InstanceResult, error) {
	if err := c.v10Login(); err != nil {
		return nil, fmt.Errorf("V10 登录失败: %w", err)
	}
	respBody, status, err := c.v10Request("GET", "/v1/host/"+upstreamID, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("V10 获取实例失败(HTTP %d)", status)
	}
	var parsed struct {
		Status int                    `json:"status"`
		Msg    string                 `json:"msg"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("解析实例详情失败: %w", err)
	}
	if parsed.Status != 200 {
		return nil, fmt.Errorf("V10 API 返回错误: %s", parsed.Msg)
	}
	return parseInstanceResult(parsed.Data), nil
}

// Start 开机
func (c *Client) Start(upstreamID string) error {
	return c.instanceAction(upstreamID, "start", "cloudHostStart")
}

// Stop 关机
func (c *Client) Stop(upstreamID string) error {
	return c.instanceAction(upstreamID, "stop", "cloudHostStop")
}

// Reboot 重启
func (c *Client) Reboot(upstreamID string) error {
	return c.instanceAction(upstreamID, "reboot", "cloudHostReboot")
}

// Reinstall 重装系统
func (c *Client) Reinstall(upstreamID string, osID string) error {
	return c.instanceActionWithParams(upstreamID, "reinstall", "cloudHostReinstall", map[string]string{"os": osID})
}

// ResetPassword 重置密码
func (c *Client) ResetPassword(upstreamID string, password string) error {
	return c.instanceActionWithParams(upstreamID, "resetPassword", "cloudHostResetPassword", map[string]string{"password": password})
}

// Delete 销毁实例
func (c *Client) Delete(upstreamID string) error {
	return c.instanceAction(upstreamID, "delete", "cloudHostDelete")
}

// instanceAction 执行无参数的实例操作
func (c *Client) instanceAction(upstreamID, v10Action, legacyAction string) error {
	if c.cfg.AuthType == "v10" {
		if err := c.v10Login(); err != nil {
			return err
		}
		_, status, err := c.v10Request("POST", "/v1/host/"+upstreamID+"/"+v10Action, nil)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("V10 %s 失败(HTTP %d)", v10Action, status)
		}
		return nil
	}
	_, err := c.call(c.defaultAction(v10Action, legacyAction), map[string]interface{}{"id": upstreamID})
	return err
}

// instanceActionWithParams 执行带参数的实例操作
func (c *Client) instanceActionWithParams(upstreamID, v10Action, legacyAction string, extra map[string]string) error {
	if c.cfg.AuthType == "v10" {
		if err := c.v10Login(); err != nil {
			return err
		}
		body := map[string]interface{}{}
		for k, v := range extra {
			if v != "" {
				body[k] = v
			}
		}
		_, status, err := c.v10Request("POST", "/v1/host/"+upstreamID+"/"+v10Action, body)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("V10 %s 失败(HTTP %d)", v10Action, status)
		}
		return nil
	}
	params := map[string]interface{}{"id": upstreamID}
	for k, v := range extra {
		if v != "" {
			params[k] = v
		}
	}
	_, err := c.call(c.defaultAction(v10Action, legacyAction), params)
	return err
}

// GetConsole 获取控制台/VNC 信息
func (c *Client) GetConsole(upstreamID string) (*ConsoleInfo, error) {
	if c.cfg.AuthType == "v10" {
		if err := c.v10Login(); err != nil {
			return nil, err
		}
		respBody, status, err := c.v10Request("GET", "/v1/host/"+upstreamID+"/console", nil)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("V10 获取控制台失败(HTTP %d)", status)
		}
		var parsed struct {
			Status int                    `json:"status"`
			Msg    string                 `json:"msg"`
			Data   map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, fmt.Errorf("解析控制台响应失败: %w", err)
		}
		if parsed.Status != 200 {
			return nil, fmt.Errorf("V10 API 返回错误: %s", parsed.Msg)
		}
		ci := &ConsoleInfo{Raw: parsed.Data}
		ci.Type = stringField(parsed.Data, "type", "vnc")
		ci.URL = stringField(parsed.Data, "url", "vnc_url", "console_url")
		ci.Host = stringField(parsed.Data, "host", "ip", "address")
		ci.Port = intField(parsed.Data, "port", "vnc_port")
		ci.Password = stringField(parsed.Data, "password", "vnc_password")
		return ci, nil
	}

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

// ---------- 辅助解析 ----------

func extractList(data map[string]interface{}, keys ...string) []map[string]interface{} {
	for _, k := range keys {
		if raw, ok := data[k]; ok {
			if arr, ok := raw.([]interface{}); ok {
				out := make([]map[string]interface{}, 0, len(arr))
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						out = append(out, m)
					}
				}
				return out
			}
			if arr, ok := raw.([]map[string]interface{}); ok {
				return arr
			}
		}
	}
	if arr, ok := data[""].([]map[string]interface{}); ok {
		return arr
	}
	return []map[string]interface{}{}
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

// maskURL 脱敏 URL：保留 scheme+host，隐藏 path/query
func maskURL(raw string) string {
	if idx := strings.Index(raw, "://"); idx > 0 {
		rest := raw[idx+3:]
		if slashIdx := strings.Index(rest, "/"); slashIdx > 0 {
			return raw[:idx+3] + rest[:slashIdx] + "/***"
		}
		return raw
	}
	return raw
}

// truncStr 截断字符串到指定长度
func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// unused 避免未使用导入告警
var _ = bytes.MinRead
