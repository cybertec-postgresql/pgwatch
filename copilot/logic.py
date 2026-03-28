"""
pgwatch3 AI Copilot - Core Logic Module
Consolidated logic for handling gPRC metric updates and Gemini AI analysis.
"""

import os
import argparse
import grpc
from concurrent import futures
from typing import Any
import pgwatch_pb2
import pgwatch_pb2_grpc
from google import genai
from dotenv import load_dotenv

load_dotenv()

API_KEY = os.getenv("GEMINI")
GEMINI_MODEL = "gemini-2.0-flash"

client = genai.Client(api_key=API_KEY)

class CopilotReceiver(pgwatch_pb2_grpc.ReceiverServicer):
    """
    gRPC Servicer that receives PostgreSQL metrics and provides 
    AI-driven insights using Google Gemini.
    """
    def __init__(self, tui_app=None):
        self.tui_app = tui_app

    def _log(self, message: str):
        """Helper to log either to TUI or Console"""
        if self.tui_app:
            self.tui_app.log_to_ui(message)
        else:
            print(message)

    def UpdateMeasurements(self, request: pgwatch_pb2.MeasurementEnvelope, context: grpc.ServicerContext) -> pgwatch_pb2.Reply:
        """
        Receives real-time metrics and generates an AI report (WHAT/WHEN/WHY/HOW).
        """
        db_name = request.DBName
        metric_name = request.MetricName

        if request.Data:
            first_point = request.Data[0]
            prompt = self._build_prompt(db_name, metric_name, first_point)

            try:
                self._log(f"[bold yellow]Analyzing {metric_name} for {db_name}...[/]")
                
                if not API_KEY or API_KEY == "your_api_key_here":
                    response_text = f"MOCK REPORT: {metric_name} is showing issues on {db_name}. (API Key Missing)"
                else:
                    try:
                        response = client.models.generate_content(
                            model=GEMINI_MODEL,
                            contents=prompt
                        )
                        response_text = response.text
                    except Exception as ai_err:
                        self._log(f"[bold red][AI Quota/Model Error] {ai_err}[/]")
                        # Access struct fields safely
                        status = "unknown"
                        if metric_name in first_point.fields:
                            status = str(first_point.fields[metric_name])
                        response_text = f"MOCK REPORT (Fallback): {metric_name} is showing status {status} on {db_name}. (AI API unavailable: {type(ai_err).__name__})"
                
                self._log(f"\n[bold green]--- AI REPORT ---[/]\n{response_text}\n[bold green]------------------[/]\n")
            except Exception as e:
                self._log(f"[bold red][Error] {e}[/]")
        return pgwatch_pb2.Reply(logmsg="Copilot analysis complete.")

    def _build_prompt(self, db_name: str, metric_name: str, data_point: Any) -> str:
        """Helper to construct the expert prompt"""
        return f"""Act as a PostgreSQL Expert. Explain complex metrics clearly and actionably.
        Database: {db_name}
        Metric: {metric_name}
        Data point: {data_point}

        Please provide:
        1. WHAT: What is happening right now with this metric?
        2. WHEN: When did it happen?
        3. WHY: Why is this happening? (e.g., High CPU, Memory leak, etc.)
        4. HOW: How can I fix it? (e.g., SQL query, Config change, etc.)
        
        Keep the response concise and under 100 words.
        """

    def SyncMetric(self, request: Any, context: grpc.ServicerContext) -> pgwatch_pb2.Reply:
        self._log("[bold blue][Copilot] Syncing metrics...[/]")
        return pgwatch_pb2.Reply(logmsg="Metrics synced.")

    def DefineMetrics(self, request: Any, context: grpc.ServicerContext) -> pgwatch_pb2.Reply:
        self._log("[bold magenta][Copilot] Defining metrics...[/]")
        return pgwatch_pb2.Reply(logmsg="Metrics defined.")

def serve(host: str = "[::]", port: int = 50051):
    """Starts the gRPC server with configurable host and port."""
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    pgwatch_pb2_grpc.add_ReceiverServicer_to_server(CopilotReceiver(), server)
    
    address = f"{host}:{port}"
    server.add_insecure_port(address)
    print(f"🚀 AI Copilot is listening on {address}...")

    server.start()
    server.wait_for_termination()

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="pgwatch3 AI copilot Backend")
    parser.add_argument("--host", type=str, default="[::]", help="Host interface to bind to (default: [::])")
    parser.add_argument("--port", type=int, default=50051, help="Port to listen on (default: 50051)")
    args = parser.parse_args()

    serve(host=args.host, port=args.port)

