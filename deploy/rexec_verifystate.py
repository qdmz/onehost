import paramiko, os

HOST = "38.55.132.191"
USER = "root"
PASS = "thanks123A#"
SQL_LOCAL = os.path.join(os.path.dirname(__file__), "verify_state.sql")
SQL_REMOTE = "/tmp/verify_state.sql"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(HOST, 22, USER, PASS, timeout=30)

sftp = ssh.open_sftp()
sftp.put(SQL_LOCAL, SQL_REMOTE)
sftp.close()

def run(cmd):
    stdin, stdout, stderr = ssh.exec_command(cmd)
    out = stdout.read().decode(errors="replace")
    err = stderr.read().decode(errors="replace")
    return out, err

out, err = run("mysql oneclickvirt < " + SQL_REMOTE)
print("=== DB STATE ===")
print(out)
if err.strip():
    print("ERR:", err)

print("=== HEALTH (8080) ===")
h, _ = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health")
print(h.strip())

print("=== FORGOT-PASSWORD ===")
fp, _ = run("curl -s -X POST http://127.0.0.1:8080/api/v1/auth/forgot-password -H 'Content-Type: application/json' -d '{\"email\":\"admin@ypvps.com\"}'")
print(fp.strip())

ssh.close()
