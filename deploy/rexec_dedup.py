import paramiko, os, time

HOST="38.55.132.191"; USER="root"; PASS="thanks123A#"
DEP=os.path.dirname(__file__)

ssh=paramiko.SSHClient(); ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
ssh.connect(HOST,22,USER,PASS,timeout=30)

def run(cmd):
    i,o,e=ssh.exec_command(cmd); return o.read().decode(errors="replace"), e.read().decode(errors="replace")

def run_sql_file(local, remote="/tmp/step.sql"):
    sftp=ssh.open_sftp(); sftp.put(local, remote); sftp.close()
    out,err=run("mysql -N -B oneclickvirt < "+remote)
    return out, err

# 0) write SELECT to file (avoid backtick-in-shell issues)
with open(os.path.join(DEP,"_sel.sql"),"w") as f:
    f.write("SELECT id, category, `key`, value, UNIX_TIMESTAMP(updated_at) FROM system_configs WHERE deleted_at IS NULL;\n")
out,err=run_sql_file(os.path.join(DEP,"_sel.sql"))
if err.strip():
    print("SELECT ERR:", err); ssh.close(); raise SystemExit(1)

rows=[]
for line in out.splitlines():
    line=line.rstrip("\n")
    if not line.strip(): continue
    parts=line.split("\t")
    if len(parts)<5: continue
    rid=int(parts[0]); cat=parts[1]; key=parts[2]; val=parts[3]; upd=float(parts[4] or 0)
    rows.append((rid,cat,key,val,upd))

print("LIVE ROWS FETCHED:", len(rows))

# 1) compute keeper per (category,key): prefer non-empty value, then latest updated_at, then max id
groups={}
for rid,cat,key,val,upd in rows:
    gk=cat+"\x00"+key
    g=groups.get(gk)
    r_nonempty = (val != "")
    if g is None:
        groups[gk]=[rid,val,upd]; continue
    g_nonempty=(g[1]!="")
    better=False
    if r_nonempty and not g_nonempty:
        better=True
    elif r_nonempty==g_nonempty:
        if upd>g[2] or (upd==g[2] and rid>g[0]):
            better=True
    if better:
        groups[gk]=[rid,val,upd]

keep=set(g[0] for g in groups.values())
del_ids=[r[0] for r in rows if r[0] not in keep]
print("DISTINCT GROUPS:", len(groups), " TO DELETE:", len(del_ids))

# 2) backup table
print("=== BACKUP ===")
out,err=run("mysql oneclickvirt -e \"DROP TABLE IF EXISTS system_configs_bak; CREATE TABLE system_configs_bak LIKE system_configs; INSERT INTO system_configs_bak SELECT * FROM system_configs;\"")
print(out, err)

# 3) delete in batches
if del_ids:
    with open(os.path.join(DEP,"_del.sql"),"w") as f:
        B=2000
        for s in range(0,len(del_ids),B):
            batch=del_ids[s:s+B]
            f.write("DELETE FROM system_configs WHERE id IN (%s);\n" % ",".join(str(x) for x in batch))
    out,err=run_sql_file(os.path.join(DEP,"_del.sql"))
    if err.strip():
        print("DELETE ERR:", err); ssh.close(); raise SystemExit(1)
    print("DELETE DONE")

# 4) fix unique index -> (category, key)
with open(os.path.join(DEP,"_idx.sql"),"w") as f:
    f.write("ALTER TABLE system_configs DROP INDEX IF EXISTS idx_system_configs_cat_key;\n")
    f.write("ALTER TABLE system_configs ADD UNIQUE INDEX idx_system_configs_cat_key (category, `key`);\n")
print("=== FIX INDEX ===")
out,err=run_sql_file(os.path.join(DEP,"_idx.sql"))
print(out, err)
if err.strip():
    print("INDEX ERR:", err)

# 5) verify
with open(os.path.join(DEP,"_chk.sql"),"w") as f:
    f.write("SELECT COUNT(*) FROM system_configs WHERE deleted_at IS NULL;\n")
    f.write("SELECT COUNT(DISTINCT CONCAT(category,'#',`key`)) FROM system_configs WHERE deleted_at IS NULL;\n")
    f.write("SELECT `key`, COUNT(*) FROM system_configs WHERE `key` LIKE 'auth.email-%' GROUP BY `key`;\n")
out,err=run_sql_file(os.path.join(DEP,"_chk.sql"))
print("=== VERIFY AFTER FIX ===")
print(out, err)

# 6) restart service
print("=== RESTART ===")
out,err=run("/bin/systemctl restart oneclickvirt")
print(out, err)
time.sleep(3)

print("=== HEALTH ===")
h,_=run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/health"); print(h.strip())
print("=== FORGOT-PASSWORD ===")
fp,_=run("curl -s -X POST http://127.0.0.1:8080/api/v1/auth/forgot-password -H 'Content-Type: application/json' -d '{\"email\":\"admin@ypvps.com\"}'")
print(fp.strip())

ssh.close()
