package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/examples/demodata"
	"github.com/catamat/wsrpc/fiberws"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
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

func main() {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Define an upgrader to handle upgrading HTTP connections to WebSocket.
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// Handler for the WebSocket connection.
	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		defer c.Close()

		// Wrap the WebSocket connection using fiberws.Conn
		conn := fiberws.NewAdapter(c)
		config := wsrpc.DefaultConfig()
		config.LogOutput = io.Discard

		// Create the WSRPC server
		wsrpcServer, err := wsrpc.NewServer(conn, config)
		if err != nil {
			log.Printf("Error creating server: %v\n", err)
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
	}))

	fmt.Println("Server listening on :50505")
	err := app.Listen(":50505")
	if err != nil {
		log.Println("Error starting HTTP server:", err)
	}
}
