package main

import (
	"errors"
	"log"

	"github.com/cybertec-postgresql/pgwatch/v3/api"
	"github.com/destrex271/pgwatch3_rpc_server/sinks"
)

// AuthenticatedWrapper wraps a Receiver with authentication
type AuthenticatedWrapper struct {
	Receiver sinks.Receiver
	Token    string
}

// Create a new authenticated wrapper for a receiver
func NewAuthenticatedWrapper(receiver sinks.Receiver, token string) *AuthenticatedWrapper {
	return &AuthenticatedWrapper{
		Receiver: receiver,
		Token:    token,
	}
}

// Implement the UpdateMeasurements method with authentication
func (wrapper *AuthenticatedWrapper) UpdateMeasurements(req *sinks.AuthRequest, logMsg *string) error {
	// Verify token
	if req.Token != wrapper.Token {
		*logMsg = "Authentication failed: invalid token"
		log.Println("[ERROR]: Authentication failed - invalid token")
		return errors.New("authentication failed: invalid token")
	}

	// Extract the actual measurement envelope
	msg, ok := req.Data.(api.MeasurementEnvelope)
	if !ok {
		*logMsg = "Invalid data format"
		log.Println("[ERROR]: Invalid data format")
		return errors.New("invalid data format")
	}

	// Forward to the actual receiver
	return wrapper.Receiver.UpdateMeasurements(&msg, logMsg)
}

// Implement the SyncMetric method with authentication
func (wrapper *AuthenticatedWrapper) SyncMetric(req *sinks.AuthRequest, logMsg *string) error {
	// Verify token
	if req.Token != wrapper.Token {
		*logMsg = "Authentication failed: invalid token"
		log.Println("[ERROR]: Authentication failed - invalid token")
		return errors.New("authentication failed: invalid token")
	}

	// Extract the actual sync request
	syncReq, ok := req.Data.(api.RPCSyncRequest)
	if !ok {
		*logMsg = "Invalid data format"
		log.Println("[ERROR]: Invalid data format")
		return errors.New("invalid data format")
	}

	// Forward to the actual receiver
	return wrapper.Receiver.SyncMetric(&syncReq, logMsg)
}
