#!/usr/bin/env python3
"""Verify a Compose database via streaming dump/restore, then remove the check DB."""
import argparse
import re
import secrets
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--database", default="goiosvm", help="source database (default: goiosvm)")
args = parser.parse_args()
if not re.fullmatch(r"[a-z][a-z0-9_]*", args.database):
    parser.error("database must be a simple PostgreSQL identifier")

prefix = ["docker", "compose", "exec", "-T", "postgres"]
name = "restore_check_" + secrets.token_hex(5)

def sql(database, query):
    return subprocess.check_output(prefix + ["psql", "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-U", "goiosvm", "-d", database, "-c", query])

# These cover all published ownership relationships and exact chunk bytes.
queries = [
    "SELECT row_to_json(x)::text FROM (SELECT * FROM artifact ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM firmware_target ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM analysis_run ORDER BY id) x",
    "SELECT id || ':' || encode(sha256(data),'hex') FROM evidence_chunk ORDER BY id",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM blob_chunk ORDER BY blob_id,ordinal) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM observation ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM finding ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM finding_revision ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM finding_basis ORDER BY revision_id,role,observation_id,reference_revision_id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM finding_assessment_event ORDER BY revision_id,id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM operation_progress ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM operation_progress_event ORDER BY operation_id,sequence) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM acquisition ORDER BY id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM run_observation ORDER BY run_id,observation_id) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM run_batch ORDER BY run_id,component,ordinal) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM run_checkpoint ORDER BY run_id,component) x",
    "SELECT row_to_json(x)::text FROM (SELECT * FROM run_coverage ORDER BY run_id,component) x",
]
subprocess.run(prefix + ["createdb", "-U", "goiosvm", name], check=True)
try:
    before = [sql(args.database, q) for q in queries]
    dump = subprocess.Popen(prefix + ["pg_dump", "-U", "goiosvm", "-d", args.database, "-Fc"], stdout=subprocess.PIPE)
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
    print("Backup/restore preserved supporting bytes, finding revisions, basis relationships and assessment history")
finally:
    subprocess.run(prefix + ["dropdb", "-U", "goiosvm", name], check=True)
