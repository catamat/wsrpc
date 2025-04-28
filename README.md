# WSRPC
[![License](https://img.shields.io/github/license/mashape/apistatus.svg)](https://github.com/catamat/wsrpc/blob/master/LICENSE)
[![Build Status](https://travis-ci.org/catamat/wsrpc.svg?branch=master)](https://travis-ci.org/catamat/wsrpc)
[![Go Report Card](https://goreportcard.com/badge/github.com/catamat/wsrpc)](https://goreportcard.com/report/github.com/catamat/wsrpc)
[![Go Reference](https://pkg.go.dev/badge/github.com/catamat/wsrpc.svg)](https://pkg.go.dev/github.com/catamat/wsrpc)
[![Version](https://img.shields.io/github/tag/catamat/wsrpc.svg?color=blue&label=version)](https://github.com/catamat/wsrpc/releases)

WSRPC is simple package to allow bidirectional RPC over a WebSocket connection.\
The library already provides WebSocket adapters for Gorilla, Fiber, and FastHTTP; for all other cases, it should be quite easy to implement the `Conn` interface.

## Installation:
```
go get github.com/catamat/wsrpc@latest
```
## Examples:

### Gorilla server
```golang
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
```

### Fiber server
```golang
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
```

### Gorilla client
```golang
package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/examples/demodata"
	"github.com/catamat/wsrpc/gorillaws"
	"github.com/gorilla/websocket"
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

	// Wrap the WebSocket connection using gorilla.Conn
	conn := gorillaws.NewAdapter(wsConn)
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
```

### FastHTTP client
```golang
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
```