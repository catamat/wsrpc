# WSRPC
[![License](https://img.shields.io/github/license/mashape/apistatus.svg)](https://github.com/catamat/wsrpc/blob/master/LICENSE)
[![Build Status](https://travis-ci.org/catamat/wsrpc.svg?branch=master)](https://travis-ci.org/catamat/wsrpc)
[![Go Report Card](https://goreportcard.com/badge/github.com/catamat/wsrpc)](https://goreportcard.com/report/github.com/catamat/wsrpc)
[![Go Reference](https://pkg.go.dev/badge/github.com/catamat/wsrpc.svg)](https://pkg.go.dev/github.com/catamat/wsrpc)
[![Version](https://img.shields.io/github/tag/catamat/wsrpc.svg?color=blue&label=version)](https://github.com/catamat/wsrpc/releases)

WSRPC is simple package to allow bidirectional RPC over a WebSocket connection.\
The library already provides WebSocket adapters for Gorilla, Fiber, and FastHTTP; for all other cases, it should be quite easy to implement the `WSConn` interface.

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
	"log"
	"net/http"

	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/gorillaws"
	"github.com/gorilla/websocket"
)

// Definition of the API that the client can call on the server.
type ServerAPI struct{}

func (s *ServerAPI) Hello(args string, reply *string) error {
	*reply = "I am the server " + args
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
		log.Fatalf("Error upgrading connection to WebSocket: %v", err)
		return
	}
	defer wsConn.Close()

	// Wrap the WebSocket connection using gorillaws.Conn
	wsAdapter := &gorillaws.Conn{Conn: wsConn}

	// Create server connection
	serverConn := wsrpc.NewWebSocketConn(wsAdapter)

	// Create the WSRPC server
	wsrpcServer, err := wsrpc.NewServer(serverConn)
	if err != nil {
		log.Fatalf("Error creating server: %v", err)
		return
	}
	defer wsrpcServer.Close()

	// Register the service on the server for calls from the client
	err = wsrpcServer.Register(&ServerAPI{})
	if err != nil {
		log.Fatalf("Error registering service on server: %v", err)
		return
	}

	// Server calls the Hello method on the client
	var reply string
	err = wsrpcServer.Call("ClientAPI.Hello", "called by server", &reply)
	if err != nil {
		log.Fatalf("Error in RPC call from server to client: %v", err)
		return
	}

	log.Println("Response from client:", reply)

	// Wait until the connection is closed
	<-wsrpcServer.Done()
}

func main() {
	http.HandleFunc("/ws", wsHandler)

	fmt.Println("Server listening on :50505")
	err := http.ListenAndServe(":50505", nil)
	if err != nil {
		log.Fatal("Error starting HTTP server:", err)
	}
}
```

### Fiber server
```golang
package main

import (
	"fmt"
	"log"

	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/fiberws"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// Definition of the API that the client can call on the server.
type ServerAPI struct{}

func (s *ServerAPI) Hello(args string, reply *string) error {
	*reply = "I am the server " + args
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
		wsAdapter := &fiberws.Conn{Conn: c}

		// Create server connection
		serverConn := wsrpc.NewWebSocketConn(wsAdapter)

		// Create the WSRPC server
		wsrpcServer, err := wsrpc.NewServer(serverConn)
		if err != nil {
			log.Fatalf("Error creating server: %v", err)
			return
		}
		defer wsrpcServer.Close()

		// Register the service on the server for calls from the client
		err = wsrpcServer.Register(&ServerAPI{})
		if err != nil {
			log.Fatalf("Error registering service on server: %v", err)
			return
		}

		// Server calls the Hello method on the client
		var reply string
		err = wsrpcServer.Call("ClientAPI.Hello", "called by server", &reply)
		if err != nil {
			log.Fatalf("Error in RPC call from server to client: %v", err)
			return
		}

		log.Println("Response from client:", reply)

		// Wait until the connection is closed
		<-wsrpcServer.Done()
	}))

	fmt.Println("Server listening on :50505")
	err := app.Listen(":50505")
	if err != nil {
		log.Fatal("Error starting HTTP server:", err)
	}
}
```

### Gorilla client
```golang
package main

import (
	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/gorillaws"
	"github.com/gorilla/websocket"
	"log"
	"time"
)

// Definition of the API that the server can call on the client.
type ClientAPI struct{}

func (c *ClientAPI) Hello(args string, reply *string) error {
	*reply = "I am the client " + args
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
	wsAdapter := &gorillaws.Conn{Conn: wsConn}

	// Create client connection
	clientConn := wsrpc.NewWebSocketConn(wsAdapter)

	// Create the WSRPC client
	wsrpcClient, err := wsrpc.NewClient(clientConn)
	if err != nil {
		log.Printf("Error creating client: %v", err)
		return err
	}
	defer wsrpcClient.Close()

	// Register the service on the client for calls from the server
	err = wsrpcClient.Register(&ClientAPI{})
	if err != nil {
		log.Fatalf("Error registering service on client: %v", err)
		return err
	}

	// Client calls the Hello method on the server
	var reply string
	err = wsrpcClient.Call("ServerAPI.Hello", "called by client", &reply)
	if err != nil {
		log.Fatalf("Error in RPC call from client to server: %v", err)
		return err
	}

	log.Println("Response from server:", reply)

	// Wait until the connection is closed
	<-wsrpcClient.Done()

	return nil
}
```

### FastHTTP client
```golang
package main

import (
	"log"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/catamat/wsrpc"
	"github.com/catamat/wsrpc/fasthttpws"
)

// Definition of the API that the server can call on the client.
type ClientAPI struct{}

func (c *ClientAPI) Hello(args string, reply *string) error {
	*reply = "I am the client " + args
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
	wsAdapter := &fasthttpws.Conn{Conn: wsConn}

	// Create client connection
	clientConn := wsrpc.NewWebSocketConn(wsAdapter)

	// Create the WSRPC client
	wsrpcClient, err := wsrpc.NewClient(clientConn)
	if err != nil {
		log.Printf("Error creating client: %v", err)
		return err
	}
	defer wsrpcClient.Close()

	// Register the service on the client for calls from the server
	err = wsrpcClient.Register(&ClientAPI{})
	if err != nil {
		log.Fatalf("Error registering service on client: %v", err)
		return err
	}

	// Client calls the Hello method on the server
	var reply string
	err = wsrpcClient.Call("ServerAPI.Hello", "called by client", &reply)
	if err != nil {
		log.Fatalf("Error in RPC call from client to server: %v", err)
		return err
	}

	log.Println("Response from server:", reply)

	// Wait until the connection is closed
	<-wsrpcClient.Done()

	return nil
}
```