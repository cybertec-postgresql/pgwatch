package pwctl

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// checks if the port is active
func isPortActive(server_address string) bool {
	resp, err := http.Get("http://" + server_address)
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// gets the port from the server address
func getPort(server_address string) (string, error) {
	index := strings.LastIndex(server_address, ":")
	if index == -1 {
		return "", errors.New("invalid server address: no port number")
	}
	return server_address[index+1:], nil
}

// checks if the pgwatch3 server is live in the docker container
func IsServerLive(server_address string) bool {
	if !isPortActive(server_address) {
		return false
	}
	port, err := getPort(server_address)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	cmd := exec.Command("docker", "ps", "--filter", "name=pgwatch3", "--format", "{{.Ports}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	output := out.String()
	// fmt.Println(output) // for debugging
	return strings.Contains(output, "0.0.0.0:"+port+"->")
}
