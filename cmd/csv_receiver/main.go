package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"net/rpc"
)

func main() {
	// Important Flags
	port := flag.String("port", "-1", "Specify the port where you want your sink to receive the measurements on.")
	storageFolder := flag.String("rootFolder", ".", "Only for formats like CSV...\n")
	token := flag.String("token", "", "Authentication token")
	flag.Parse()

	if *port == "-1" {
		log.Println("[ERROR]: No Port Specified")
		return
	}

	// Create the actual CSV receiver
	actualReceiver := NewCSVReceiver(*storageFolder)
	log.Println("[INFO]: CSV Receiver Initialized")

	var server interface{}

	// If token is provided, wrap with authentication
	if *token != "" {
		log.Println("[INFO]: Authentication enabled")
		server = NewAuthenticatedWrapper(actualReceiver, *token)
	} else {
		log.Println("[WARNING]: No authentication token provided - running in insecure mode")
		server = actualReceiver
	}

	rpc.RegisterName("Receiver", server) // Primary Receiver
	log.Println("[INFO]: Registered Receiver")
	rpc.HandleHTTP()

	listener, err := net.Listen("tcp", "0.0.0.0:"+*port)
	if err != nil {
		log.Fatal(err)
	}

	http.Serve(listener, nil)
}
