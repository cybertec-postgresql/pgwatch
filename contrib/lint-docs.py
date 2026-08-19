#!/usr/bin/env python3
"""
Diátaxis quadrant lint for pgwatch docs.

Walks every .md file under docs/, classifies it by its folder (tutorial/,
howto/, concept/, reference/, gallery/, intro/, developer/), and verifies:

1. Every file has YAML frontmatter with a `title:` field.
2. Every file mentioned in mkdocs.yml nav points at a file that exists.
3. Every file under docs/{tutorial,howto,concept,reference,gallery} appears
   in the mkdocs.yml nav (i.e. is reachable from the published site).
4. Cross-link convention (S7): every concept/, tutorial/, and howto/ file
   links to at least one file in another quadrant via a relative path,
   so the four quadrants interlock rather than drift into isolation.

The lint does not attempt to detect mixed-mode content — it relies on the
author to place each file in the correct folder. Frontend enforcement is
deliberately minimal: we trust the folder mapping and surface only the
mechanical invariants (frontmatter presence, nav reachability, cross-links).

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
INTERLOCK_FOLDERS = {"tutorial", "howto", "concept"}


def list_doc_files() -> list[Path]:
    return sorted(p for p in DOCS.rglob("*.md") if p.is_file())


def extract_nav_targets() -> set[str]:
    """Return the set of .md paths (relative to docs/) referenced from mkdocs.yml nav."""
    text = MKDOCS.read_text(encoding="utf-8")
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


def cross_quadrants(path: Path) -> set[str]:
    """Return the set of *quadrant folders* referenced by `path`.

    Walks every `](file.md)` link, resolves it relative to the source file,
    and collects the first path component. This makes sibling-quadrant
    navigation (`../howto/foo.md`) and same-quadrant navigation
    (`./sibling.md`) detectable the same way.
    """
    text = path.read_text(encoding="utf-8")
    out: set[str] = set()
    src_dir = path.parent
    for m in re.finditer(r"\]\(([^)#]+\.md)(?:#[^)]*)?\)", text):
        raw = m.group(1).replace("\\", "/")
        if raw.startswith(("http://", "https://", "mailto:")):
            continue
        try:
            target = (src_dir / raw).resolve()
            rel = target.relative_to(DOCS.resolve()).as_posix()
        except (ValueError, OSError):
            continue
        parts = rel.split("/")
        if parts and parts[0] in {"tutorial", "howto", "concept", "reference", "gallery"}:
            out.add(parts[0])
    return out


def main() -> int:
    files = list_doc_files()
    nav = extract_nav_targets()
    errors: list[str] = []

    for f in files:
        rel = f.relative_to(DOCS).as_posix()

        if not has_frontmatter_title(f):
            errors.append(f"{rel}: missing YAML frontmatter title")

        parts = f.relative_to(DOCS).parts
        if parts[0] in DOC_FOLDERS and rel not in nav:
            errors.append(f"{rel}: not reachable from mkdocs.yml nav")

        # S7 cross-link convention: concept/, tutorial/, howto/ files should
        # link to at least one *different* quadrant.
        if parts[0] in INTERLOCK_FOLDERS:
            targets = cross_quadrants(f)
            others = targets - {parts[0]}
            if not others:
                errors.append(
                    f"{rel}: must link to a file in a different quadrant "
                    f"(found quadrants: {sorted(targets) or 'none'})"
                )

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
