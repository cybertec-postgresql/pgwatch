# pgwatch3 AI Copilot

This directory contains the AI-driven Copilot for pgwatch3, implemented in Python with a gRPC backend and a Terminal User Interface (TUI).

## Components
- **`tui.py`**: The Terminal User Interface built with [Textual](https://textual.textualize.io/).
- **`logic.py`**: The gRPC backend and AI logic (Google Gemini).
- **`pgwatch_pb2.py` / `pgwatch_pb2_grpc.py`**: Generated gRPC code.

## How to Run
1. Ensure your environment variables are set in `.env` (requires `GEMINI` API key).
2. Start the AI Copilot:
   ```bash
   python copilot/tui.py
   ```
3. (Optional) Run the test client to simulate metrics:
   ```bash
   python copilot/test_client.py
   ```
