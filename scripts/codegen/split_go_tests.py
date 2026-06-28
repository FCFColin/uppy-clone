#!/usr/bin/env python3
"""Split a Go test file into multiple files by test function name patterns."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

FUNC_RE = re.compile(r"^func (?:\([^)]*\) )?(\w+)\(", re.MULTILINE)


def extract_functions(source: str) -> list[tuple[str, str]]:
    matches = list(FUNC_RE.finditer(source))
    funcs: list[tuple[str, str]] = []
    for i, m in enumerate(matches):
        name = m.group(1)
        start = m.start()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(source)
        funcs.append((name, source[start:end].rstrip() + "\n"))
    return funcs


def pick_target(name: str, rules: list[tuple[str, list[str]]], default: str) -> str:
    for target, patterns in rules:
        for pat in patterns:
            if re.search(pat, name):
                return target
    return default


def split_file(
    src: Path,
    rules: list[tuple[str, list[str]]],
    default: str,
    header: str,
    dry_run: bool = False,
) -> dict[str, list[str]]:
    source = src.read_text(encoding="utf-8")
    funcs = extract_functions(source)
    buckets: dict[str, list[str]] = {}
    for name, body in funcs:
        if not (name.startswith("Test") or name.startswith("Benchmark")):
            continue
        target = pick_target(name, rules, default)
        buckets.setdefault(target, []).append(body)

    out_dir = src.parent
    for target, bodies in buckets.items():
        out_path = out_dir / target
        content = header + "\n\n" + "\n".join(bodies)
        if dry_run:
            print(f"{out_path}: {len(bodies)} functions")
        else:
            out_path.write_text(content, encoding="utf-8")
            print(f"wrote {out_path} ({len(bodies)} functions)")
    return buckets


ROOM_RULES = [
    (
        "room_broadcast_test.go",
        [
            r"Broadcast",
            r"SendToPlayer",
            r"GetConnection",
            r"BuildSnapshot",
        ],
    ),
    (
        "room_persist_test.go",
        [r"SaveState"],
    ),
    (
        "room_tick_test.go",
        [
            r"HandleMessage",
            r"ValidateTap",
            r"DecodeTap",
            r"UpdatePlayerStats",
        ],
    ),
    (
        "room_lifecycle_test.go",
        [
            r"Room_",
            r"HandleSetNickname",
            r"HandleRestart",
            r"CheckRestart",
            r"Restart",
            r"TryStart",
            r"CleanupDisconnected",
            r"AllConnected",
            r"TransitionPhase",
            r"NormalizePhase",
            r"ModelPhase",
        ],
    ),
]

MISC_RULES = [
    ("physics_test.go", [r"ApplyPhysics", r"ApplyTap", r"CheckGhost", r"CheckBird", r"UpdateGhost", r"UpdateBird", r"ApplyGhostRepel", r"CalculateCooldown", r"GenerateRoomCode", r"BenchmarkCalculate"]),
    ("names_test.go", [r"GenerateRandom", r"GenerateUnique", r"SanitizePlayerName", r"RandomIndex", r"TrimSpace"]),
    ("broadcaster_test.go", [r"Broadcaster", r"Broadcast", r"HandleRemote", r"NilBroadcaster"]),
    ("state_test.go", [r"NewGameState", r"ResetGameEntities", r"Serialize", r"Deserialize", r"BenchmarkSerialize", r"BenchmarkDeserialize", r"BenchmarkNewGameState"]),
    ("repository_test.go", [r"RoomRepository", r"SnapshotEncoder", r"MockRoom", r"MockSnapshot"]),
]

ROOM_HEADER = """package game

import (
\t"errors"
\t"log/slog"
\t"math"
\t"os"
\t"testing"
\t"time"

\t"github.com/uppy-clone/backend/internal/config"
\t"github.com/uppy-clone/backend/internal/domain"
\t"github.com/uppy-clone/backend/internal/protocol"
)"""

MISC_HEADER = """package game

import (
\t"context"
\t"errors"
\t"fmt"
\t"math"
\t"regexp"
\t"strings"
\t"sync"
\t"testing"
\t"time"

\t"github.com/uppy-clone/backend/internal/config"
\t"github.com/uppy-clone/backend/internal/domain"
\t"github.com/uppy-clone/backend/internal/protocol"
)"""

HANDLER_RULES = [
    ("ws_handler_test.go", [r"WebSocket"]),
    ("auth_handler_test.go", [r"RefreshToken", r"ExportUser", r"DeleteUser", r"Logout", r"Unauthorized"]),
    ("lobby_handler_test.go", [r"CreateRoom", r"CheckRoom", r"MatchRoom"]),
    ("handler_degraded_test.go", [r"WriteDegraded", r"RequireDB", r"RequireRedis", r"RequireHub"]),
]

HANDLER_HEADER = """package handler

import (
\t"context"
\t"encoding/json"
\t"net/http"
\t"net/http/httptest"
\t"strings"
\t"sync"
\t"testing"
\t"time"

\t"github.com/go-chi/chi/v5"
\t"github.com/uppy-clone/backend/internal/auth"
\t"github.com/uppy-clone/backend/internal/config"
\t"github.com/uppy-clone/backend/internal/game"
\t"github.com/uppy-clone/backend/internal/store"
)"""

LIFECYCLE_RULES = [
    (
        "room_restart_test.go",
        [r"Restart", r"HandleRestart", r"CheckRestart"],
    ),
]

LIFECYCLE_HEADER = """package game

import (
\t"log/slog"
\t"os"
\t"testing"
\t"time"

\t"github.com/uppy-clone/backend/internal/config"
\t"github.com/uppy-clone/backend/internal/domain"
\t"github.com/uppy-clone/backend/internal/protocol"
)"""


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["room", "misc", "handler", "lifecycle"])
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    game = root / "backend" / "internal" / "game"
    if args.mode == "room":
        split_file(game / "room_test.go", ROOM_RULES, "room_test.go", ROOM_HEADER, args.dry_run)
        if not args.dry_run:
            keep = game / "room_test.go"
            keep.write_text(ROOM_HEADER + "\n", encoding="utf-8")
            print(f"trimmed {keep}")
    elif args.mode == "misc":
        split_file(game / "game_misc_test.go", MISC_RULES, "game_misc_test.go", MISC_HEADER, args.dry_run)
        if not args.dry_run:
            keep = game / "game_misc_test.go"
            keep.write_text(MISC_HEADER + "\n", encoding="utf-8")
            print(f"trimmed {keep}")
    elif args.mode == "lifecycle":
        split_file(game / "room_lifecycle_test.go", LIFECYCLE_RULES, "room_lifecycle_test.go", LIFECYCLE_HEADER, args.dry_run)
    else:
        handler = root / "backend" / "internal" / "handler"
        split_file(handler / "handler_misc_test.go", HANDLER_RULES, "handler_misc_test.go", HANDLER_HEADER, args.dry_run)
        if not args.dry_run:
            keep = handler / "handler_misc_test.go"
            keep.write_text(HANDLER_HEADER + "\n", encoding="utf-8")
            print(f"trimmed {keep}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
