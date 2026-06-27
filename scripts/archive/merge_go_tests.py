#!/usr/bin/env python3
"""Merge multiple *_test.go files in a package into fewer files (one-time maintainer tool)."""
from __future__ import annotations

import re
import sys
from pathlib import Path


def strip_header(content: str) -> str:
    """Remove package declaration and import block from test file body."""
    content = re.sub(r"^package\s+\w+\s*\n", "", content, count=1)
    content = re.sub(r"^//go:build[^\n]*\n", "", content, count=1)
    content = re.sub(r"^import\s*\([^)]*\)\s*\n", "", content, count=1, flags=re.DOTALL)
    content = re.sub(r"^import\s+[^\n]+\n", "", content, count=1)
    return content.strip() + "\n"


def merge_package(pkg_dir: Path, groups: dict[str, list[str]], header: str) -> None:
    for out_name, sources in groups.items():
        bodies: list[str] = []
        for src in sources:
            path = pkg_dir / src
            if not path.exists():
                print(f"skip missing {path}")
                continue
            bodies.append(strip_header(path.read_text(encoding="utf-8")))
        out_path = pkg_dir / out_name
        out_path.write_text(header + "\n\n" + "\n\n".join(bodies), encoding="utf-8")
        print(f"written {out_path} ({len(sources)} sources)")
        for src in sources:
            if src != out_name:
                p = pkg_dir / src
                if p.exists():
                    p.unlink()
                    print(f"deleted {p}")


def main() -> None:
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("backend/internal/game")
    header = "package game\n\nimport (\n\t\"testing\"\n\t\"time\"\n\t\"context\"\n\t\"sync\"\n\t\"net/http\"\n\t\"net/http/httptest\"\n\t\"encoding/json\"\n\t\"bytes\"\n\t\"strings\"\n\t\"log/slog\"\n\t\"errors\"\n\t\"fmt\"\n\t\"os\"\n\t\"path/filepath\"\n\n\t\"github.com/go-chi/chi/v5\"\n\t\"github.com/stretchr/testify/require\"\n\t\"github.com/alicebob/miniredis/v2\"\n\n\t\"github.com/uppy-clone/backend/internal/domain\"\n\t\"github.com/uppy-clone/backend/internal/protocol\"\n\t\"github.com/uppy-clone/backend/internal/config\"\n\t\"github.com/uppy-clone/backend/internal/metrics\"\n\t\"github.com/uppy-clone/backend/internal/idgen\"\n\t\"github.com/uppy-clone/backend/internal/testutil\"\n\t\"github.com/uppy-clone/backend/internal/store\"\n)"
    groups = {
        "hub_test.go": [
            "hub_test.go",
            "hub_limits_test.go",
            "hub_restore_test.go",
            "hub_match_test.go",
            "hub_cache_test.go",
            "hub_resolve_test.go",
        ],
        "room_test.go": [
            "room_test.go",
            "room_restart_test.go",
            "room_nickname_test.go",
            "room_trystart_test.go",
            "room_persist_test.go",
            "room_broadcast_test.go",
            "room_tick_test.go",
            "room_lifecycle_pure_test.go",
        ],
        "game_misc_test.go": [
            "broadcaster_test.go",
            "repository_test.go",
            "names_test.go",
            "state_test.go",
            "physics_test.go",
        ],
    }
    merge_package(root, groups, header)


if __name__ == "__main__":
    main()
