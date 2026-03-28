"""
pgwatch3 AI Copilot - Terminal User Interface
"""
import sys
import os 
import threading
from textual.app import App, ComposeResult
from textual.widgets import Header, Footer, RichLog
import grpc
from concurrent import futures

sys.path.append(os.path.dirname(os.path.abspath(__file__)))

try: 
    from logic import CopilotReceiver
except ImportError:
    from copilot.logic import CopilotReceiver

import pgwatch_pb2_grpc

class CopilotTUI(App):
    TITLE = "pgwatch3 AI Copilot"
    BINDINGS = [("q","quit", "Quit")]

    def compose(self) -> ComposeResult:
        yield Header()

        yield RichLog(id="ai_logs", highlight=True, markup=True)
        yield Footer()
    
    def on_mount(self) -> None:
        self.log_to_ui("🚀 [bold green] TUI Started![/] Waiting for metrics from pgwatch3...")
        threading.Thread(target=self.run_grpc_server, daemon=True).start()
    
    def run_grpc_server(self):
        try:
            server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
            pgwatch_pb2_grpc.add_ReceiverServicer_to_server(CopilotReceiver(tui_app=self), server)
            
            # Using 0.0.0.0 to listen on all interfaces (more compatible on some Windows setups)
            port = "50051"
            server.add_insecure_port(f"0.0.0.0:{port}")
            server.start()
            self.log_to_ui(f"✅ [bold blue]gRPC Server Listening[/] on port {port}")
            server.wait_for_termination()
        except Exception as e:
            self.log_to_ui(f"❌ [bold red]gRPC Server Error:[/] {e}")
    
    def log_to_ui(self, message: str) -> None:
        """
        Thread-safe method to update the UI. Checks if called from main thread or background.
        """
        log_widget = self.query_one("#ai_logs", RichLog)
        if self._thread_id == threading.get_ident():
            log_widget.write(message)
        else:
            self.call_from_thread(log_widget.write, message)

if __name__ == "__main__":
    app = CopilotTUI()
    app.run()        