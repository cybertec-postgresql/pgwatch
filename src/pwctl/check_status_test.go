package pwctl

import (
	"strings"
	"testing"
)

func TestIsPortActive(t *testing.T) {
	// Test with a server address that you know is active in the pgwatch docker container
	if !isPortActive("localhost:8080") {
		t.Error("Expected server to be active, but it was not")
	}
	// Test with a server address that you know is not active in the pgwatch docker container
	if !isPortActive("localhost:3000") {
		t.Error("Expected server to not be active, but it was")
	}

	// Test with a server address that you know is not active
	if isPortActive("localhost:9999") {
		t.Error("Expected server to not be active, but it was")
	}
}

func TestGetPort(t *testing.T) {
	// Test with a server address that has a port
	port, err := getPort("localhost:8080")
	if err != nil {
		t.Error("Unexpected error:", err)
	} else if port != "8080" {
		t.Error("Expected port to be 8080, but got", port)
	}

	// Test with a server address that does not have a port
	_, err = getPort("localhost")
	if err == nil {
		t.Error("Expected error, but got none")
	} else if !strings.Contains(err.Error(), "invalid server address: no port number") {
		t.Error("Expected 'invalid server address: no port number' error, but got", err)
	}
}

func TestIsServerLive(t *testing.T) {
	// Test with a server address that you know is live in the pgwatch docker container
	if !IsServerLive("localhost:8080") {
		t.Error("Expected server to be live, but it was not")
	}
	// Test with a server address that you know is live but not in the pgwatch docker container
	if IsServerLive("localhost:5501") {
		t.Error("Expected server to be live, but it was not")
	}

	// Test with a server address that you know is not live
	if IsServerLive("localhost:9999") {
		t.Error("Expected server to not be live, but it was")
	}
}
