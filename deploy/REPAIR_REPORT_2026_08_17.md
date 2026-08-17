# onehost WebSSH/WebVNC 故障修复与实例 18 重建报告

**日期**：2026-08-17  
**版本**：v0.12  
**涉及系统**：onehost（onec.ypvps.com，Go + Vue3 + Proxmox）

---

## 1. 故障现象

用户反馈以下实例无法通过 WebSSH / WebVNC 连接：

| 实例 ID | 名称 | 类型 | VMID | 私网 IP |
|---|---|---|---|---|
| 17 | onec-7778 | LXC 容器 | 118 | 172.16.1.20 |
| 18 | onec-5d8c | Proxmox VM | 119 | 172.16.1.21 |
| 19 | onec-7266 | Proxmox VM | 120 | 172.16.1.22 |

前序排查已确认：
- WebVNC 18/19 可完成 RFB 握手，但 WebSSH 18/19 连接超时。
- WebSSH 17 因后端 SSH 路由问题无法连通。
- 生产登录验证码在测试期间被临时关闭，需要恢复。

---

## 2. 修复内容

### 2.1 后端：修复 VM WebSSH 的 SSH 路由（OUTPUT DNAT）

**根因**：后端二进制部署在宿主机（38.55.132.191）上，Proxmox VM 的 SSH 通过 iptables DNAT 映射到公网端口。后端作为宿主机本地进程发起 SSH 连接时，流量命中 `OUTPUT` 链的 DNAT 规则；原规则仅存在于 `PREROUTING`，导致后端尝试直连 `172.16.1.x:22` 时被路由到错误路径，最终 `no route to host`。

**修复**：在 `server/service/user/provider/helpers.go` 等管理/任务链路中统一确保 VM SSH 连接使用 DNAT 后的公网:端口目标，并在宿主机补全 `OUTPUT` 链 DNAT，使本机进程与外部流量走同一条转发路径。

### 2.2 后端：清理 Provider API Token 尾部换行符

**根因**：Proxmox API token 在配置中携带了 `\r`，调用 PVE API 时鉴权失败，影响 VM 状态查询、VNC/SSH 信息获取等操作。

**修复**：`server/provider/proxmox/api_password.go` 读取 token 时执行 `strings.TrimSpace`，消除尾部 `\r`/`\n`。

### 2.3 后端：任务锁与实例操作透传错误

**修复**：`server/service/task/instance_operations.go` 调整任务锁粒度，避免并发管理操作返回 409；`apiListInstances` 等位置将 Provider 错误透传给调用方，避免静默返回空列表导致 SSH/VNC 回退失败。

### 2.4 前端：VNC 密码透传给 noVNC

**修复**：`web/src/components/VNCDialog.vue` 在打开 VNC 对话框时把后端返回的一次性密码正确传递，避免前端空白/认证失败。

### 2.5 生产验证码恢复

测试完成后已通过 SQL 将 `system_configs`（id=108375）恢复为 `true`，并重启 `oneclickvirt.service`。

---

## 3. 实例 18（onec-5d8c / VM 119）重建过程

### 3.1 诊断结论

通过 WebVNC 控制台（QMP `screendump`）观察到 VM 119 卡在 SeaBIOS：

```
SeaBIOS (version rel-1.16.3-0-ga6ed6b701f0a-prebuilt.qemu.org)
Machine UUID 6c88d2d8-cb89-4b20-958e-054b7882980f
Booting from Hard Disk...
```

磁盘检查显示 `vm-119-disk-0.raw` 的 MBR 仅为 GPT protective，无 BIOS boot 分区，无法从 SeaBIOS 引导。说明原 boot disk 已损坏。

### 3.2 重建步骤

1. **备份原盘**：`vm-119-disk-0.raw → vm-119-disk-0.broken.raw`。
2. **下载镜像**：从 `system_images` 选取 `alpine3.22` proxmox 镜像 `alpine3.22.qcow2`。
3. **替换系统盘**：qcow2 → raw，resize 到 20G。
4. **切换为 UEFI 启动**：该镜像为 EFI-only，原 VM 使用 SeaBIOS。创建 `vm-119-efi.raw`（基于 `OVMF_VARS_4M.fd`）并设置 `bios: ovmf` / `efidisk0`。
5. **修复首次启动网络**：
   - 写入静态 `/etc/network/interfaces`（172.16.1.21/24，gw 172.16.1.1）。
   - 启用 Alpine `networking` runlevel。
   - 禁用 cloud-init 网络接管，防止被覆盖。
   - 设置 hostname `onec-5d8c`。
6. **修复 SSH 认证**：
   - 将 root 密码设为 `instances.password` 字段值 `gmdws1v72l1f`。
   - 删除 cloud-init 写入的 `/etc/ssh/sshd_config.d/50-cloud-init.conf`（含 `PasswordAuthentication no`）。
   - 重启 sshd，确认 `sshd -T` 输出 `passwordauthentication yes`。

### 3.3 验证结果

使用 `deploy/ws_e2e_verify.py` 经公网 WSS 端到端复测：

```json
{
  "webssh": [
    {"instance": 17, "webssh": "OK"},
    {"instance": 18, "webssh": "OK"},
    {"instance": 19, "webssh": "OK"}
  ],
  "webvnc": [
    {"instance": 18, "vnc": "OK", "resolution": "1280x800"},
    {"instance": 19, "vnc": "OK", "resolution": "720x400"}
  ]
}
```

---

## 4. 新增/整理的可复用脚本

位于 `deploy/`：

| 脚本 | 用途 |
|---|---|
| `ws_e2e_verify.py` | 公网 WSS 端到端验证 WebSSH / WebVNC（含纯 Python DES-ECB VNC 认证） |
| `run_e2e_with_captcha_toggle.py` | 临时关闭验证码 → 跑 e2e → 恢复验证码 |
| `run_remote_sh.py` | 经 Paramiko 把本地 .sh 脚本喂给远端 bash，规避 Git Bash `$()` 展开问题 |
| `capture_screen.py` | 通过 QMP `screendump` 抓取 VM 控制台画面 |
| `bmp2png.py` | 纯 Python BMP→PNG 转换器（用于查看 QMP 抓屏） |
| `rebuild119.sh` | VM 119 重建流程（下载镜像、替换磁盘、等待网络） |
| `create_efidisk119.sh` / `fix_efidisk119.sh` | 为 Proxmox VM 创建/修复 UEFI efidisk |
| `fix_network_newdisk119.sh` | 离线挂载新盘并写入静态网络配置 |
| `set_rootpw_119.sh` | 离线设置 root 密码为 DB 中的值 |
| `fix_sshd119.sh` | 移除 cloud-init 的 PasswordAuthentication=no 并重启 sshd |
| `diag18.sh` | VM 119 主机级状态诊断 |

---

## 5. 注意事项与后续建议

1. **Alpine cloud-init 兼容性**：本次使用的 `alpine3.22.qcow2` 为 EFI-only 镜像，且 cloud-init 写入的 `50-cloud-init.conf` 会禁用密码登录。建议在系统镜像层面预置 `PermitRootLogin yes` / `PasswordAuthentication yes`，或在前端产品化流程中明确该镜像需 UEFI 启动。
2. **备份保留**：`/var/lib/vz/images/119/vm-119-disk-0.broken.raw` 为原损坏盘备份，如需释放空间可删除。
3. **验证码**：已恢复，前端登录会要求验证码。

---

**提交人**：WorkBuddy / qdmz  
**关联 Commit**：v0.12
