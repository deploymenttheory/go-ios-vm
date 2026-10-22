#!/usr/bin/env python3
"""Verify the dedicated Compose database via streaming dump/restore, then remove the check DB."""
import secrets
import subprocess

prefix = ["docker", "compose", "exec", "-T", "postgres"]
name = "restore_check_" + secrets.token_hex(5)

def sql(database, query):
    return subprocess.check_output(prefix + ["psql", "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-U", "goiosvm", "-d", database, "-c", query])

# These cover all published ownership relationships and exact chunk bytes.
queries = [
    "SELECT id || ':' || encode(sha256(data),'hex') FROM evidence_chunk ORDER BY id",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM blob_chunk ORDER BY blob_id,ordinal) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM observation ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM finding_revision ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM run_observation ORDER BY run_id,observation_id) x",
]
subprocess.run(prefix + ["createdb", "-U", "goiosvm", name], check=True)
try:
    before = [sql("goiosvm", q) for q in queries]
    dump = subprocess.Popen(prefix + ["pg_dump", "-U", "goiosvm", "-d", "goiosvm", "-Fc"], stdout=subprocess.PIPE)
    try:
        restored = subprocess.run(prefix + ["pg_restore", "-U", "goiosvm", "-d", name, "--no-owner", "--exit-on-error"], stdin=dump.stdout)
        dump.stdout.close()
        if dump.wait() or restored.returncode:
            raise SystemExit("Database backup/restore failed")
    finally:
        if dump.poll() is None:
            dump.kill()
            dump.wait()
    after = [sql(name, q) for q in queries]
    if before != after:
        raise SystemExit("Restored evidence bytes or relationships differ")
    print("Backup/restore preserved supporting bytes and publication relationships")
finally:
    subprocess.run(prefix + ["dropdb", "-U", "goiosvm", name], check=True)
