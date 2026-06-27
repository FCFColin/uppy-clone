#!/usr/bin/env python3
"""Merge *_test.go files in a package into grouped output files."""
from __future__ import annotations

import re
import sys
from pathlib import Path


def strip_header(content: str) -> str:
    content = re.sub(r"^package\s+\w+\s*\n", "", content, count=1)
    content = re.sub(r"^//go:build[^\n]*\n", "", content, count=1)
    content = re.sub(r"^import\s*\([^)]*\)\s*\n", "", content, count=1, flags=re.DOTALL)
    content = re.sub(r"^import\s+[^\n]+\n", "", content, count=1)
    return content.strip() + "\n"


def merge(pkg_dir: Path, groups: dict[str, list[str]], header: str) -> None:
    for out_name, sources in groups.items():
        bodies = []
        for src in sources:
            p = pkg_dir / src
            if not p.exists():
                continue
            bodies.append(strip_header(p.read_text(encoding="utf-8")))
        (pkg_dir / out_name).write_text(header + "\n\n" + "\n\n".join(bodies), encoding="utf-8")
        for src in sources:
            if src != out_name and (pkg_dir / src).exists():
                (pkg_dir / src).unlink()


def main() -> None:
    root = Path(sys.argv[1])
    pkg = root.name
    header = f"package {pkg}\n"
    if pkg == "auth":
        groups = {
            "auth_token_test.go": ["jwt_test.go", "refresh_test.go", "revoke_test.go"],
            "auth_flow_test.go": ["magiclink_test.go", "quickplay_test.go"],
            "auth_misc_test.go": ["gdpr_data_test.go", "secure_test.go", "outbox_events_test.go", "middleware_test.go"],
        }
    elif pkg == "middleware":
        groups = {
            "middleware_test.go": [
                "cors_test.go", "logging_test.go", "ratelimit_test.go", "tracing_test.go",
                "prometheus_test.go", "security_test.go", "bulkhead_test.go", "proxy_test.go", "idempotency_test.go",
            ],
        }
    elif pkg == "handler":
        groups = {
            "auth_test.go": ["auth_test.go"],
            "admin_test.go": ["admin_test.go", "admin_password_test.go"],
            "handler_misc_test.go": ["lobby_test.go", "websocket_test.go", "degradation_test.go", "handler_extra_test.go"],
        }
    elif pkg == "store":
        groups = {
            "postgres_test.go": [
                "postgres_test.go", "postgres_users_test.go", "postgres_lobbies_query_test.go",
                "postgres_env_test.go", "env_helpers_test.go",
            ],
        }
    else:
        print("unknown package")
        return
    merge(root, groups, header)
    print(f"merged {pkg}")


if __name__ == "__main__":
    main()
