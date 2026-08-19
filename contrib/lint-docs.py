#!/usr/bin/env python3
"""
Diátaxis quadrant lint for pgwatch docs.

Walks every .md file under docs/, classifies it by its folder (tutorial/,
howto/, concept/, reference/, gallery/, intro/, developer/), and verifies:

1. Every file has YAML frontmatter with a `title:` field.
2. Every file mentioned in mkdocs.yml nav points at a file that exists.
3. Every file under docs/{tutorial,howto,concept,reference,gallery} appears
   in the mkdocs.yml nav (i.e. is reachable from the published site).

The lint does not attempt to detect mixed-mode content — it relies on the
author to place each file in the correct folder. Frontend enforcement is
deliberately minimal: we trust the folder mapping and surface only the
mechanical invariants (frontmatter presence, nav reachability).

Exits 0 on success, 1 on any violation.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "docs"
MKDOCS = ROOT / "mkdocs.yml"
DOC_FOLDERS = {"tutorial", "howto", "concept", "reference", "gallery"}


def list_doc_files() -> list[Path]:
    return sorted(p for p in DOCS.rglob("*.md") if p.is_file())


def extract_nav_targets() -> set[str]:
    """Return the set of .md paths (relative to docs/) referenced from mkdocs.yml nav."""
    text = MKDOCS.read_text(encoding="utf-8")
    # Match lines of the form "    - Label: foo/bar.md" — nav entries written
    # by hand. Capture the path component only.
    targets: set[str] = set()
    for m in re.finditer(r"^\s+-\s+[^:]+:\s*((?:[\w-]+/)*[\w.-]+\.md)\s*$", text, re.MULTILINE):
        targets.add(m.group(1).replace("\\", "/"))
    return targets


def has_frontmatter_title(path: Path) -> bool:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---\n"):
        return False
    end = text.find("\n---", 4)
    if end < 0:
        return False
    return bool(re.search(r"^title:\s*\S", text[4:end], re.MULTILINE))


def main() -> int:
    files = list_doc_files()
    nav = extract_nav_targets()
    errors: list[str] = []

    for f in files:
        # Both sides relative to docs/ so the comparison works.
        rel = f.relative_to(DOCS).as_posix()

        if not has_frontmatter_title(f):
            errors.append(f"{rel}: missing YAML frontmatter title")

        parts = f.relative_to(DOCS).parts
        if parts[0] in DOC_FOLDERS and rel not in nav:
            errors.append(f"{rel}: not reachable from mkdocs.yml nav")

    # Nav entries pointing at non-existent files.
    for n in nav:
        if (DOCS / n).is_file():
            continue
        errors.append(f"{n}: listed in nav but file does not exist")

    if errors:
        print("docs lint: FAILED")
        for e in errors:
            print(f"  - {e}")
        return 1

    print(f"docs lint: OK ({len(files)} files checked)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
