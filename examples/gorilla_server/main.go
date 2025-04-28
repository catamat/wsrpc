package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/examples/demodata"
	"github.com/catamat/wsrpc/gorillaws"
	"github.com/gorilla/websocket"
)

// Definition of the API that the client can call on the server.
type ServerAPI struct{}

func (s *ServerAPI) Hello(args string, reply *string) error {
	log.Printf("Server: Received ServerAPI.Hello call with args: %q\n", args)
	*reply = "Server says hello back to: " + args
	return nil
}

func (s *ServerAPI) DelayedHello(args string, reply *string) error {
	log.Printf("Server: Received ServerAPI.DelayedHello call with args: %q - Waiting 10 seconds...\n", args)
	time.Sleep(10 * time.Second)
	*reply = "Server belatedly says hello back to: " + args
	log.Println("Server: Responding to ServerAPI.DelayedHello")
	return nil
}

func (s *ServerAPI) LargePayload(args []byte, reply *int) error {
	payloadLen := len(args)
	log.Printf("Server: Received ServerAPI.LargePayload call with payload size: %d bytes\n", payloadLen)
	*reply = payloadLen
	return nil
}

// Define an upgrader to handle upgrading HTTP connections to WebSocket.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Handler for the WebSocket connection.
func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection to a WebSocket connection
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading connection to WebSocket: %v\n", err)
		return
	}
	defer wsConn.Close()

	// Wrap the WebSocket connection using gorillaws.Conn
	conn := gorillaws.NewAdapter(wsConn)
	config := wsrpc.DefaultConfig()
	config.LogOutput = io.Discard

	// Create the WSRPC server
	wsrpcServer, err := wsrpc.NewServer(conn, config)
	if err != nil {
		return
	}
	defer wsrpcServer.Close()

	// Register the service on the server for calls from the client
	err = wsrpcServer.Register(&ServerAPI{})
	if err != nil {
		log.Printf("Error registering service on server: %v\n", err)
		return
	}

	// --- Server calls Client ---

	// 1. Hello
	log.Println("Server: Calling ClientAPI.Hello...")

	var clientReplyHello string
	err = wsrpcServer.Call("ClientAPI.Hello", "server initial call", &clientReplyHello)
	if err != nil {
		log.Printf("Server: Error calling ClientAPI.Hello: %v\n", err)
	} else {
		log.Printf("Server: Response from ClientAPI.Hello: %q\n", clientReplyHello)
	}

	// 2. DelayedHello
	log.Println("Server: Calling ClientAPI.DelayedHello (will block)...")

	var clientReplyDelayed string
	err = wsrpcServer.Call("ClientAPI.DelayedHello", "server delayed call", &clientReplyDelayed)
	if err != nil {
		log.Printf("Server: Error calling ClientAPI.DelayedHello: %v\n", err)
	} else {
		log.Printf("Server: Response from ClientAPI.DelayedHello: %q\n", clientReplyDelayed)
	}

	// 3. LargePayload
	log.Println("Server: Calling ClientAPI.LargePayload...")

	largeArgs := []byte(demodata.LargePayload)
	var clientReplyLarge int
	err = wsrpcServer.Call("ClientAPI.LargePayload", largeArgs, &clientReplyLarge)
	if err != nil {
		log.Printf("Server: Error calling ClientAPI.LargePayload: %v\n", err)
	} else {
		log.Printf("Server: Response from ClientAPI.LargePayload (client received size): %d bytes\n", clientReplyLarge)
	}

	// ------

	// Wait until the connection is closed
	<-wsrpcServer.Done()
}

func main() {
	http.HandleFunc("/ws", wsHandler)

	fmt.Println("Server listening on :50505")
	err := http.ListenAndServe(":50505", nil)
	if err != nil {
		log.Println("Error starting HTTP server:", err)
	}
}
