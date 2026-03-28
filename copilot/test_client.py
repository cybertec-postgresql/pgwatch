import os
import grpc
import argparse
from dotenv import load_dotenv
import pgwatch_pb2
import pgwatch_pb2_grpc
from google.protobuf import struct_pb2

# Load environment to get the same settings as the server
load_dotenv()

def run_test(host: str = "127.0.0.1", port: int = 50051):
    address = f"{host}:{port}"
    print(f"Connecting to AI Copilot at {address}...")
    
    with grpc.insecure_channel(address) as channel:
        stub = pgwatch_pb2_grpc.ReceiverStub(channel)

        # 1. Create a fake "Measurement" (Simulating 95% CPU load)
        data_row = struct_pb2.Struct()
        data_row.update({
            "load_1min": 0.95,
            "active_connections": 120,
            "waiting_queries": 15
        })

        # 2. Package it into the Envelope
        envelope = pgwatch_pb2.MeasurementEnvelope(
            DBName="SLIIT_Project_DB",
            MetricName="cpu_load",
            Data=[data_row]
        )

        print(f"Sending fake metric: cpu_load=0.95 to Copilot on {address}...")
        
        try:
            response = stub.UpdateMeasurements(envelope)
            print(f"Server Response: {response.logmsg}")
        except Exception as e:
            print(f"RPC Failed: {e}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="pgwatch3 AI Copilot Test Client")
    parser.add_argument("--host", type=str, default="127.0.0.1", help="Target host (default: 127.0.0.1)")
    parser.add_argument("--port", type=int, default=50051, help="Target port (default: 50051)")
    args = parser.parse_args()

    run_test(host=args.host, port=args.port)
    