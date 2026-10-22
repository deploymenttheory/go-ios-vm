#!/usr/bin/env python3
"""Create local-only database credentials without committing or printing secrets."""
import os
from pathlib import Path
import secrets

path = Path(__file__).resolve().parent.parent / ".env"
password = secrets.token_hex(24)
text = (
    f"POSTGRES_PASSWORD={password}\n"
    "POSTGRES_PORT=55432\n"
    f"GOIOSVM_DATABASE_URL=postgres://goiosvm:{password}@127.0.0.1:55432/goiosvm?sslmode=disable\n"
)
try:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    raise SystemExit(".env already exists; existing credentials were preserved")
with os.fdopen(fd, "w") as output:
    output.write(text)
print("Created local .env with mode 0600")
