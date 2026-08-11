import paramiko
HOST="38.55.132.191"; USER="root"; PASS="thanks123A#"
ssh=paramiko.SSHClient(); ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(HOST,22,USER,PASS,timeout=30)
def run(cmd):
    i,o,e=ssh.exec_command(cmd); return o.read().decode(errors="replace"), e.read().decode(errors="replace")
out,err=run("mysql -N -e \"SELECT VERSION();\"")
print("MARIADB VERSION:", out.strip())
out,err=run("mysql oneclickvirt -N -e \"SELECT COUNT(*) FROM system_configs WHERE deleted_at IS NULL;\"")
print("LIVE ROWS:", out.strip())
ssh.close()
