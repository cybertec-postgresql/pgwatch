import os
import grpc
from google import genai
from dotenv import load_dotenv
import pgwatch_pb2
import pgwatch_pb2_grpc

load_dotenv()

API_KEY = os.getenv("GEMINI")
client = genai.Client(api_key=API_KEY)

# Set this to False to use the real Gemini API. Set to True for internal UI testing without quota.
MOCK_AI = False 

class CopilotReceiver(pgwatch_pb2_grpc.ReceiverServicer):
    def __init__(self, tui_app=None):
        self.tui_app = tui_app

    def UpdateMeasurements(self, request, context):
        db_name = request.DBName
        metric_name = request.MetricName
        
        if request.Data:
            first_point = request.Data[0]
            prompt = f"""Act as you are a PostgreSQL Expert who can explain the complex database metrics to developers in a clear, actional way. 
            Database Details:
            Database: {db_name}
            Metric: {metric_name}
            Data point: {first_point}
            
            Please provide:
            1.WHAT: What is happening right now with this metric?
            2.WHY : Why is this occuring? Explain the technical root cause.
            3.HOW : How can the developer fix this? Provide a specific SQL command or config change."""
            try:
                msg = f"Copilot is analyzing {metric_name} for {db_name}..."
                print(msg)
                if self.tui_app:
                    self.tui_app.call_from_thread(self.tui_app.query_one("#ai_logs").write, f"[yellow]AI:[/] {msg}")

                if MOCK_AI:
                    # Simulation for local TUI development
                    report = f"[[MOCK REPORT FOR {metric_name.upper()}]]\n" \
                             "**WHAT:** High CPU detected! (Mocked analysis)\n" \
                             "**WHY:** This is a simulated response for testing the TUI layout.\n" \
                             "**HOW:** No action needed, everything is fine in mock mode!\n"
                else:
                    response = client.models.generate_content(model="gemini-2.0-flash", contents=prompt)
                    report = response.text
                
                print(f"AI REPORT:\n{report}")
                
                if self.tui_app:
                    self.tui_app.call_from_thread(self.tui_app.query_one("#ai_logs").write, f"\n[bold green]AI ANALYSIS REPORT:[/]\n{report}\n")
                
                return pgwatch_pb2.Reply(logmsg=f"Analysis for {metric_name} complete.")
            except Exception as e:
                err_msg = f"AI Error: {e}"
                print(err_msg)
                if self.tui_app:
                    self.tui_app.call_from_thread(self.tui_app.query_one("#ai_logs").write, f"[bold red]ERROR:[/] {err_msg}")
                return pgwatch_pb2.Reply(logmsg=f"Error during analysis: {e}")
        
        return pgwatch_pb2.Reply(logmsg="No data to analyze.")

    def SyncMetric(self, request, context):
        print(f"Syncing metric: {request.MetricName}")
        return pgwatch_pb2.Reply(logmsg="Metrics synced.")

    def DefineMetrics(self, request, context):
        print("Defining new metrics...")
        return pgwatch_pb2.Reply(logmsg="Metrics defined.")
