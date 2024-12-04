package fiberws

import (
	"net"
	"testing"
	"time"

	"github.com/catamat/wsrpc"
	fwebsocket "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	gwebsocket "github.com/gorilla/websocket"
)

// Definition of the service for testing.
type TestService struct{}

// Arguments and reply for the Add method.
type Args struct {
	A, B int
}

type Reply struct {
	Sum int
}

// Add method to sum two numbers.
func (s *TestService) Add(args *Args, reply *Reply) error {
	reply.Sum = args.A + args.B
	return nil
}

func TestFiberWS(t *testing.T) {
	// Create the test server
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Set up the "/ws" route
	app.Get("/ws", fwebsocket.New(func(c *fwebsocket.Conn) {
		defer c.Close()

		// Wrap the WebSocket connection using fiberws.Conn
		wsConn := &Conn{Conn: c}

		// Create server connection
		serverConn := wsrpc.NewWebSocketConn(wsConn)

		// Create the WSRPC server
		wsrpcServer, err := wsrpc.NewServer(serverConn)
		if err != nil {
			t.Errorf("Error creating WSRPC server: %v", err)
			return
		}
		defer wsrpcServer.Close()

		// Register the service on the server for calls from the client
		err = wsrpcServer.Register(&TestService{})
		if err != nil {
			t.Errorf("Error registering service on server: %v", err)
			return
		}

		// Server calls the Add method on the client
		args2 := &Args{A: 10, B: 15}
		reply2 := &Reply{}
		err = wsrpcServer.Call("TestService.Add", args2, reply2)
		if err != nil {
			t.Errorf("Error in RPC call from server to client: %v", err)
			return
		}

		if reply2.Sum != 25 {
			t.Errorf("Expected result 25, got %d", reply2.Sum)
		}

		// Wait until the connection is closed
		<-wsrpcServer.Done()
	}))

	// Channel to signal when the server is ready
	serverReady := make(chan struct{})

	// Create a net.Listener on a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Error creating listener: %v", err)
	}
	defer listener.Close()

	// Start the Fiber app in a goroutine
	go func() {
		// Signal that the server is ready
		close(serverReady)

		// Start the server
		if err := app.Listener(listener); err != nil {
			t.Errorf("Error starting Fiber server: %v", err)
		}
	}()

	// Wait for the server to be ready
	<-serverReady

	// Give the server some time to start
	time.Sleep(100 * time.Millisecond)

	// Convert the server's URL to a WebSocket URL and append "/ws"
	wsURL := "ws://" + listener.Addr().String() + "/ws"

	// Create the client
	wsConn, _, err := gwebsocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Error connecting to server: %v", err)
	}
	defer wsConn.Close()

	// Create client connection
	clientConn := wsrpc.NewWebSocketConn(wsConn)

	// Create the WSRPC client
	wsrpcClient, err := wsrpc.NewClient(clientConn)
	if err != nil {
		t.Fatalf("Error creating client: %v", err)
	}
	defer wsrpcClient.Close()

	// Register the service on the client for calls from the server
	err = wsrpcClient.Register(&TestService{})
	if err != nil {
		t.Fatalf("Error registering service on client: %v", err)
	}

	// Client calls the Add method on the server
	args := &Args{A: 5, B: 7}
	reply := &Reply{}
	err = wsrpcClient.Call("TestService.Add", args, reply)
	if err != nil {
		t.Fatalf("Error in RPC call from client to server: %v", err)
	}

	if reply.Sum != 12 {
		t.Errorf("Expected result 12, got %d", reply.Sum)
	}
}
