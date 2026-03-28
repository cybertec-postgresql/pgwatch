import os
import grpc
from dotenv import load_dotenv
import pgwatch_pb2
import pgwatch_pb2_grpc
from google.protobuf import struct_pb2

# Load environment to get the same settings as the server
load_dotenv()

def run_test():
    print("Connecting to AI Copilot at 127.0.0.1:50051...")
    with grpc.insecure_channel('127.0.0.1:50051') as channel:
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

        print("Sending fake metric: cpu_load=0.95 to Copilot...")
        
        try:
            response = stub.UpdateMeasurements(envelope)
            print(f"Server Response: {response.logmsg}")
        except Exception as e:
            print(f"RPC Failed: {e}")

if __name__ == "__main__":
    run_test()