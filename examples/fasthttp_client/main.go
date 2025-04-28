package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/examples/demodata"
	"github.com/catamat/wsrpc/fasthttpws"
	"github.com/fasthttp/websocket"
)

// Definition of the API that the server can call on the client.
type ClientAPI struct{}

func (c *ClientAPI) Hello(args string, reply *string) error {
	log.Printf("Client: Received ClientAPI.Hello call with args: %q\n", args)
	*reply = "Client says hello back to: " + args
	return nil
}

func (c *ClientAPI) DelayedHello(args string, reply *string) error {
	log.Printf("Client: Received ClientAPI.DelayedHello call with args: %q - Waiting 10 seconds...\n", args)
	time.Sleep(10 * time.Second)
	*reply = "Client belatedly says hello back to: " + args
	log.Println("Client: Responding to ClientAPI.DelayedHello")
	return nil
}

func (c *ClientAPI) LargePayload(args []byte, reply *int) error {
	payloadLen := len(args)
	log.Printf("Client: Received ClientAPI.LargePayload call with payload size: %d bytes\n", payloadLen)
	*reply = payloadLen
	return nil
}

func main() {
	retryDelay := 10 * time.Second

	for {
		err := connect()
		if err != nil {
			log.Println("Error connecting to server:", err)

			// Wait before attempting to reconnect
			time.Sleep(retryDelay)
			continue
		}
	}
}

func connect() error {
	// Connect to the server
	wsConn, _, err := websocket.DefaultDialer.Dial("ws://localhost:50505/ws", nil)
	if err != nil {
		return err
	}
	defer wsConn.Close()
	log.Println("Successfully connected to the server")

	// Wrap the WebSocket connection using fasthttpws.Conn
	conn := fasthttpws.NewAdapter(wsConn)
	config := wsrpc.DefaultConfig()
	config.LogOutput = io.Discard

	// Create the WSRPC client
	wsrpcClient, err := wsrpc.NewClient(conn, config)
	if err != nil {
		log.Printf("Error creating client: %v\n", err)
		return err
	}
	defer wsrpcClient.Close()

	// Register the service on the client for calls from the server
	err = wsrpcClient.Register(&ClientAPI{})
	if err != nil {
		log.Printf("Error registering service on client: %v\n", err)
		return err
	}

	// --- Client calls Server ---

	// 1. Hello
	log.Println("Client: Calling ServerAPI.Hello...")

	var serverReplyHello string
	err = wsrpcClient.Call("ServerAPI.Hello", "client initial call", &serverReplyHello)
	if err != nil {
		log.Printf("Client: Error calling ServerAPI.Hello: %v\n", err)
		return fmt.Errorf("ServerAPI.Hello call error: %w", err)
	} else {
		log.Printf("Client: Response from ServerAPI.Hello: %q\n", serverReplyHello)
	}

	// 2. DelayedHello
	log.Println("Client: Calling ServerAPI.DelayedHello (will block)...")

	var serverReplyDelayed string
	err = wsrpcClient.Call("ServerAPI.DelayedHello", "client delayed call", &serverReplyDelayed)
	if err != nil {
		log.Printf("Client: Error calling ServerAPI.DelayedHello: %v\n", err)
		return fmt.Errorf("ServerAPI.DelayedHello call error: %w", err)
	} else {
		log.Printf("Client: Response from ServerAPI.DelayedHello: %q\n", serverReplyDelayed)
	}

	// 3. LargePayload
	log.Println("Client: Calling ServerAPI.LargePayload...")

	largeArgs := []byte(demodata.LargePayload)
	var serverReplyLarge int
	err = wsrpcClient.Call("ServerAPI.LargePayload", largeArgs, &serverReplyLarge)
	if err != nil {
		log.Printf("Client: Error calling ServerAPI.LargePayload: %v\n", err)
		return fmt.Errorf("ServerAPI.LargePayload call error: %w", err)
	} else {
		log.Printf("Client: Response from ServerAPI.LargePayload (server received size): %d bytes\n", serverReplyLarge)
	}

	// ------

	// Wait until the connection is closed
	<-wsrpcClient.Done()

	return nil
}
