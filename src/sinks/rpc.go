/*
*
* RPC sink implementation for pgwatch3.
* Requires the address and port of the sink.
*
 */

package sinks

import (
	"context"
	"log"
	"net/rpc"
    "os"
    "strconv"

	"github.com/cybertec-postgresql/pgwatch3/metrics"
)

type RPCWriter struct {
	ctx     context.Context
	address string
	client  *rpc.Client
}

func NewRPCWriter(ctx context.Context, address string) (*RPCWriter, error) {

	client, err := rpc.DialHTTP("tcp", address)
	if err != nil {
		return nil, err
	}

	rw := &RPCWriter{
		ctx:     ctx,
		address: address,
		client:  client,
	}
	go rw.watchCtx()
	return rw, nil
}

// Sends Measurement Message to RPC Sink
func (rw *RPCWriter) Write(msgs []metrics.MeasurementMessage) error {
	if rw.ctx.Err() != nil {
		return rw.ctx.Err()
	}
	if len(msgs) == 0 {
		return nil
	}
// <<<<<<< HEAD
// 	var status int
// 	err := rw.client.Call("RPCWriter.UpdateMeasurements", msgs, &status)
// 	if err != nil {
// 		return err
// Can't use this, messes up with the RPC Function signature
// =======
	for _, msg := range msgs {
		var status int
		pgwatchID := os.Getenv("pgwatchID")
		msg.CustomTags = make(map[string]string)
		if len(pgwatchID) > 0 {
			msg.CustomTags["pgwatchId"] = pgwatchID
		} else {
			msg.CustomTags["pgwatchId"] = strconv.Itoa(os.Getpid()) + "_pgwatch3" // Replaces with PID to create a pgwatchid
		}
		err := rw.client.Call("Receiver.UpdateMeasurements", &msg, &status)
		if err != nil {
			return err
		}
    }
// >>>>>>> bc59692 (Sync Metric Added; Also added tests for this)
// 	}
	return nil
}

type SyncReq struct {
	OPR        string
	DBName     string
	PgwatchID  string
	MetricName string
}

func (rw *RPCWriter) SyncMetric(dbUnique string, metricName string, op string) error {
	syncReq := new(SyncReq)

	syncReq.DBName = dbUnique
	syncReq.OPR = op
	syncReq.PgwatchID = os.Getenv("pgwatchID")
	syncReq.MetricName = metricName

	var logMsg string
	err := rw.client.Call("Receiver.SyncMetricSignal", &syncReq, &logMsg)

	if err != nil {
		return err
	}

	if len(logMsg) > 0 {
		log.Println("[Remote SINK INFO]: ", logMsg)
	}

	return nil
}

func (rw *RPCWriter) watchCtx() {
	<-rw.ctx.Done()
	rw.client.Close()
}
