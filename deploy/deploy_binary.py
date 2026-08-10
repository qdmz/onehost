"""仅更新后端二进制到远程服务器 38.55.132.191（前端 dist 不变时使用）。

流程：停止服务 -> 备份旧二进制 -> 上传新二进制 -> 启动服务。
"""
import os
import posixpath
import sys
import time

import paramiko

HOST = "38.55.132.191"
PORT = 22
USER = "root"
PASSWORD = "thanks123A#"

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BINARY = os.path.join(ROOT, "oneclickvirt-linux-amd64")

REMOTE_DIR = "/opt/oneclickvirt"
REMOTE_BIN = posixpath.join(REMOTE_DIR, "oneclickvirt")


def log(msg):
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def run(client, cmd, timeout=600, check=True):
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    code = stdout.channel.recv_exit_status()
    if check and code != 0:
        raise RuntimeError(f"命令失败({code}): {cmd}\n{out}\n{err}")
    return code, out, err


def main():
    if not os.path.exists(BINARY):
        log(f"缺少构建产物: {BINARY}")
        return 1

    log(f"二进制大小: {os.path.getsize(BINARY) / 1024 / 1024:.1f} MB")

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(HOST, port=PORT, username=USER, password=PASSWORD, timeout=30)
    sftp = client.open_sftp()

    try:
        log("停止服务...")
        run(client, "systemctl stop oneclickvirt", check=False)

        log("备份旧二进制...")
        run(
            client,
            f"[ -f {REMOTE_BIN} ] && cp {REMOTE_BIN} {REMOTE_BIN}.bak.$(date +%s) || true",
        )

        log("上传新二进制...")
        sftp.put(BINARY, REMOTE_BIN + ".new")
        run(client, f"chmod 755 {REMOTE_BIN}.new && mv {REMOTE_BIN}.new {REMOTE_BIN}")

        log("启动服务...")
        run(client, "systemctl start oneclickvirt", check=False)

        log("等待后端监听 8080（外部数据库迁移可能需要数分钟）...")
        for i in range(90):
            time.sleep(10)
            code, out, _ = run(
                client, "ss -lnt 2>/dev/null | grep -c ':8080' || true", check=False
            )
            if out.strip() not in ("", "0"):
                log(f"后端已监听 8080（{(i + 1) * 10}s）")
                break
            code, act, _ = run(client, "systemctl is-active oneclickvirt", check=False)
            if act.strip() == "failed":
                log("服务启动失败")
                _, jl, _ = run(
                    client, "journalctl -u oneclickvirt -n 30 --no-pager", check=False
                )
                print(jl)
                return 1
            if (i + 1) % 6 == 0:
                log(f"仍在等待... {(i + 1) * 10}s")
        else:
            log("等待超时（15 分钟）")
            return 1

        _, out, _ = run(
            client,
            "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/v1/public/health",
            check=False,
        )
        log(f"本地 health: {out.strip()}")
        log("二进制部署完成")
        return 0
    finally:
        sftp.close()
        client.close()


if __name__ == "__main__":
    sys.exit(main())
