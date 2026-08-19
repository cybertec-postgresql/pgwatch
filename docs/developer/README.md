---
title: Developer Documentation
---

The `developer/` quadrant contains project-internal docs: contribution policy, code of conduct, license, and any architecture / design notes that are relevant to contributors but not to end users running pgwatch.

## Quadrant policy

The `developer/` folder is **out of scope for Diátaxis quadrant enforcement**. The four core quadrants — Tutorial, How-to, Reference, Explanation — describe how a user interacts with the product. The developer quadrant describes how someone contributes to the product; applying user-facing quadrants there would over-constrain boilerplate policy documents.

`contrib/lint-docs.py` therefore:

- **Excludes** `developer/` from the frontmatter-`title` requirement (license and code of conduct files are policy text, not user-facing docs).
- **Excludes** `developer/` from the cross-link convention (S7) — these files stand alone by design.
- **Excludes** `developer/` from the nav-reachability check — `Code of Conduct` and `License` are linked from the nav but contribute nothing semantically.

## Contents

- `contributing.md` — how to submit a patch
- `CODE_OF_CONDUCT.md` — community standards
- `LICENSE.md` — BSD-3-Clause
- `README.md` — this file
