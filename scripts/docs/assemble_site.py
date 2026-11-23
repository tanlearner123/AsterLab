#!/usr/bin/env python3
"""Assemble static _site directory and build an inlined Markdown bundle."""

from __future__ import annotations

import json
import os
import shutil
import sys
from pathlib import Path


BUNDLE_DIRS = [
    "docs/guide",
]

BUNDLE_FILES = [
    "docs/guide/index.md",
]


def main() -> int:
    out_dir = Path(sys.argv[1] if len(sys.argv) > 1 else "_site").resolve()
    repo_root = Path(__file__).resolve().parents[2]

    if out_dir.exists():
        shutil.rmtree(out_dir)
    (out_dir / "docs").mkdir(parents=True, exist_ok=True)

    portal = repo_root / "scripts" / "docs" / "portal" / "index.html"
    shutil.copy2(portal, out_dir / "index.html")
    shutil.copytree(repo_root / "docs", out_dir / "docs", dirs_exist_ok=True)

    for rel in ("CONTEXT.md", "DESIGN.md", "AGENT_API.md", "README.md"):
        src = repo_root / rel
        if src.is_file():
            shutil.copy2(src, out_dir / rel)

    (out_dir / "CNAME").write_text("docs.asterlab.party\n", encoding="utf-8")
    (out_dir / ".nojekyll").write_text("", encoding="utf-8")

    # Collect core handbook, ADR, product, and SDK Markdown files into one JS payload
    # so chapter switches and Ctrl+K search execute synchronously in 0 extra round-trips.
    docs_map: dict[str, str] = {}
    for rel in BUNDLE_FILES:
        p = repo_root / rel
        if p.is_file():
            docs_map[rel] = p.read_text(encoding="utf-8")

    for d_rel in BUNDLE_DIRS:
        d = repo_root / d_rel
        if not d.is_dir():
            continue
        for md_path in sorted(d.glob("*.md")):
            rel_posix = md_path.relative_to(repo_root).as_posix()
            docs_map[rel_posix] = md_path.read_text(encoding="utf-8")

    bundle_js = "window.__ASTERLAB_DOCS__ = " + json.dumps(docs_map, ensure_ascii=False) + ";\n"
    bundle_out = out_dir / "docs-bundle.js"
    bundle_out.write_text(bundle_js, encoding="utf-8")

    print(
        f"Assembled {out_dir} with {len(docs_map)} bundled docs "
        f"(docs-bundle.js = {bundle_out.stat().st_size / 1024:.1f} KiB)"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
