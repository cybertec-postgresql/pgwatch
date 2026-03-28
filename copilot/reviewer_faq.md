# AI Copilot TUI Migration - Reviewer FAQ

This document summarizes the changes and architectural decisions made for the TUI migration. Use this to quickly answer questions from reviewers!

### 1. Why did we move from WebUI to TUI?
- **The Feedback:** Maintainers requested that the WebUI remains strictly for administrative tasks and that the AI Copilot should be a Terminal User Interface (TUI).
- **The Solution:** I refactored the entire AI feature into a standalone Go command (`internal/cmd/copilot/`). This keeps the `internal/webui` clean and provides a lightweight, CLI-focused experience for DBAs.

### 2. What is the current architecture?
We use a **Go Frontend** with a **Python Backend sidecar**:
- **TUI (Go):** Built with `bubbletea` and `lipgloss` for a high-performance, interactive terminal experience.
- **Backend (Python):** Handles the Gemini AI analysis logic and gRPC communication. This is placed in `internal/cmd/copilot/python/` for logical grouping.
- **Communication:** The TUI and the Collector talk to the Python backend via **gRPC** (port 50051).

### 3. Where are the files located?
- **Entry Point:** `copilot/tui.py`
- **Core AI Logic:** `copilot/logic.py`
- **gRPC Protobufs:** `copilot/pgwatch_pb2.py`
- **Dependencies:** `go.mod` remains base-lined (no new dependencies for the Go side in this PR).

### 4. How was the AI Policy handled?
- **Disclosure:** Every new file (`main.go`, `logic.py`) has a header explicitly disclosing the use of **Antigravity (AI)** for structural boilerplate and refactoring.
- **PR Description:** The PR description includes a full attribution block as required by `AI_POLICY.md`.
- **Ownership:** I have manually reviewed, path-corrected, and tested all generated boilerplate (like the gRPC calls) to ensure it follows the project's patterns.

### 5. What happened to the old AI button in the WebUI?
On the `feat/ai-copilot-tui` branch, the Web UI is base-lined with `upstream/master`. All previous AI buttons and drawers are **removed**, ensuring the Web UI remains "Admin Only" as requested.

### 6. How do you test it?
1. Start the Python backend: `python internal/cmd/copilot/python/tui.py`.
2. Start the TUI: `go run internal/cmd/copilot/main.go`.
3. Use `test_ai.py` in the root to send a mock metric and see the result in the TUI log window.
