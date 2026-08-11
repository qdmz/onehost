import paramiko, os, time

HOST="38.55.132.191"; USER="root"; PASS="thanks123A#"
LOCAL=os.path.join(os.path.dirname(__file__),"..","oneclickvirt-linux-amd64")
REMOTE_TMP="/tmp/oneclickvirt-linux-amd64"
BIN="/opt/oneclickvirt/oneclickvirt"
BAK="/opt/oneclickvirt/oneclickvirt.bak"

ssh=paramiko.SSHClient(); ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(HOST,22,USER,PASS,timeout=30)

def run(cmd):
    i,o,e=ssh.exec_command(cmd); return o.read().decode(errors="replace"), e.read().decode(errors="replace")

# 0) before count
before,_=run("mysql -N -B -e \"SELECT COUNT(*) FROM oneclickvirt.system_configs WHERE deleted_at IS NULL;\"")
print("ROWS BEFORE:", before.strip())

# 1) backup
print(run("cp "+BIN+" "+BAK)[0] or "(backup done)")

# 2) upload
sftp=ssh.open_sftp(); sftp.put(LOCAL, REMOTE_TMP); sftp.close()
print("uploaded")

# 3) move into place (atomic rename, safe over running binary)
out,err=run("mv "+REMOTE_TMP+" "+BIN+" && chmod 755 "+BIN)
print("install:", out, err)

# 4) restart
run("/bin/systemctl restart oneclickvirt")
print("restart issued")

# 5) poll health
ok=False
for t in range(20):
    time.sleep(1)
    h,_=run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health")
    if h.strip()=="200":
        ok=True; print("HEALTH 200 after ~%ds"%(t+1)); break
if not ok:
    print("!!! HEALTH NOT 200, rolling back !!!")
    j,_=run("journalctl -u oneclickvirt -n 40 --no-pager")
    print(j)
    run("cp "+BAK+" "+BIN+" && /bin/systemctl restart oneclickvirt")
    time.sleep(4)
    h2,_=run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health")
    print("HEALTH AFTER ROLLBACK:", h2.strip())
    ssh.close(); raise SystemExit(1)

# 6) after count (prove no duplicate growth on restart)
after,_=run("mysql -N -B -e \"SELECT COUNT(*) FROM oneclickvirt.system_configs WHERE deleted_at IS NULL;\"")
print("ROWS AFTER:", after.strip())

# 7) forgot-password
fp,_=run("curl -s -X POST http://127.0.0.1:8080/api/v1/auth/forgot-password -H 'Content-Type: application/json' -d '{\"email\":\"admin@ypvps.com\"}'")
print("FORGOT-PASSWORD:", fp.strip())

ssh.close()
