package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"oneclickvirt/global"
	"oneclickvirt/utils"

	"go.uber.org/zap"
)

// apiSetInstancePassword 通过API设置实例密码
func (p *ProxmoxProvider) apiSetInstancePassword(ctx context.Context, instanceID, password string) error {
	// 先查找实例的VMID和类型
	vmid, instanceType, err := p.findVMIDByNameOrID(ctx, instanceID)
	if err != nil {
		global.APP_LOG.Error("API查找Proxmox实例失败",
			zap.String("instanceID", instanceID),
			zap.Error(err))
		return fmt.Errorf("查找实例失败: %w", err)
	}

	// 检查实例状态
	var statusURL string
	switch instanceType {
	case "container":
		statusURL = fmt.Sprintf("https://%s:8006/api2/json/nodes/%s/lxc/%s/status/current", p.config.Host, p.node, vmid)
	case "vm":
		statusURL = fmt.Sprintf("https://%s:8006/api2/json/nodes/%s/qemu/%s/status/current", p.config.Host, p.node, vmid)
	default:
		return fmt.Errorf("未知的实例类型: %s", instanceType)
	}

	statusReq, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	if err != nil {
		return fmt.Errorf("创建状态查询请求失败: %w", err)
	}
	p.setAPIAuth(statusReq)

	statusResp, err := p.apiClient.Do(statusReq)
	if err != nil {
		return fmt.Errorf("查询实例状态失败: %w", err)
	}
	defer statusResp.Body.Close()

	var statusResponse map[string]interface{}
	if err := json.NewDecoder(statusResp.Body).Decode(&statusResponse); err != nil {
		return fmt.Errorf("解析状态响应失败: %w", err)
	}

	if data, ok := statusResponse["data"].(map[string]interface{}); ok {
		if status, ok := data["status"].(string); ok && status != "running" {
			return fmt.Errorf("实例 %s (VMID: %s) 未运行，当前状态: %s", instanceID, vmid, status)
		}
	}

	// 根据实例类型设置密码
	switch instanceType {
	case "container":
		// LXC容器 - 通过API执行命令设置密码
		return p.apiSetContainerPassword(ctx, vmid, password)
	case "vm":
		// QEMU虚拟机 - 通过API设置cloud-init密码
		return p.apiSetVMPassword(ctx, vmid, password)
	default:
		return fmt.Errorf("未知的实例类型: %s", instanceType)
	}
}

// apiSetContainerPassword 为LXC容器设置密码。
// 注意：Proxmox LXC exec API 以非 shell 方式直接执行命令，
// 不支持 `echo 'root:X' | chpasswd` 之类的管道，API 会返回 200 但实际并未改密，
// 导致上层误判成功、密码未生效。因此容器内改密统一走节点 SSH 的 `pct exec chpasswd`
// （经 base64 传递凭据，已验证可靠），与 sshSetInstancePassword 容器内路径保持一致。
func (p *ProxmoxProvider) apiSetContainerPassword(ctx context.Context, vmid, password string) error {
	// 容器改密依赖节点 SSH 执行 pct exec，确保 SSH 连接可用
	if err := p.EnsureConnection(); err != nil {
		return fmt.Errorf("容器改密需要 SSH 连接，但连接不可用: %w", err)
	}

	script := utils.BuildTempScript(utils.TempScriptConfig{
		PrimaryCmd:     buildProxmoxContainerChpasswdCommand(vmid, password),
		FallbackCmd:    buildProxmoxContainerChpasswdCommand(vmid, password),
		TimeoutSeconds: 60,
	})
	if _, err := p.sshClient.ExecuteViaTempScript(script, nil, 180*time.Second); err != nil {
		global.APP_LOG.Error("通过SSH(pct exec)设置容器密码失败",
			zap.String("vmid", vmid),
			zap.Error(err))
		return fmt.Errorf("设置容器密码失败: %w", err)
	}

	global.APP_LOG.Info("通过SSH(pct exec)成功设置容器密码", zap.String("vmid", vmid))
	return nil
}

// apiSetVMPassword 通过API为QEMU虚拟机设置密码
func (p *ProxmoxProvider) apiSetVMPassword(ctx context.Context, vmid, password string) error {
	// 使用cloud-init设置密码
	url := fmt.Sprintf("https://%s:8006/api2/json/nodes/%s/qemu/%s/config", p.config.Host, p.node, vmid)

	// 构造cloud-init密码配置
	payload := map[string]interface{}{
		"cipassword": password,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	p.setAPIAuth(req)

	resp, err := p.apiClient.Do(req)
	if err != nil {
		return fmt.Errorf("执行API请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var respData map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&respData)
		return fmt.Errorf("设置虚拟机密码失败: status %d, response: %v", resp.StatusCode, respData)
	}

	// 重启虚拟机以应用密码更改
	restartURL := fmt.Sprintf("https://%s:8006/api2/json/nodes/%s/qemu/%s/status/reboot", p.config.Host, p.node, vmid)
	restartReq, err := http.NewRequestWithContext(ctx, "POST", restartURL, nil)
	if err != nil {
		global.APP_LOG.Warn("创建重启请求失败", zap.String("vmid", vmid), zap.Error(err))
		return nil // 密码已设置，重启失败不影响
	}
	p.setAPIAuth(restartReq)

	restartResp, err := p.apiClient.Do(restartReq)
	if err != nil {
		global.APP_LOG.Warn("重启虚拟机失败", zap.String("vmid", vmid), zap.Error(err))
		return nil // 密码已设置，重启失败不影响
	}
	defer restartResp.Body.Close()

	// 等待虚拟机重启完成（最多2分钟），避免任务提前完成而VM仍在重启中
	global.APP_LOG.Debug("等待虚拟机重启完成", zap.String("vmid", vmid))
	time.Sleep(p.waitScale(10 * time.Second))
	statusPollURL := fmt.Sprintf("https://%s:8006/api2/json/nodes/%s/qemu/%s/status/current", p.config.Host, p.node, vmid)
	for i := 0; i < 22; i++ {
		time.Sleep(p.waitScale(5 * time.Second))
		pollReq, pollErr := http.NewRequestWithContext(ctx, "GET", statusPollURL, nil)
		if pollErr != nil {
			break
		}
		p.setAPIAuth(pollReq)
		pollResp, pollErr := p.apiClient.Do(pollReq)
		if pollErr != nil {
			continue
		}
		var pollData map[string]interface{}
		json.NewDecoder(pollResp.Body).Decode(&pollData)
		pollResp.Body.Close()
		if data, ok := pollData["data"].(map[string]interface{}); ok {
			if status, ok := data["status"].(string); ok && status == "running" {
				global.APP_LOG.Debug("虚拟机重启完成，已恢复运行", zap.String("vmid", vmid))
				break
			}
		}
	}

	global.APP_LOG.Info("通过API成功设置虚拟机密码并重启", zap.String("vmid", vmid))
	return nil
}
