package fiberws

import (
	"context"
	"fmt"
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
	serverErrCh := make(chan error, 1)
	listenerErrCh := make(chan error, 1)

	reportServerErr := func(err error) {
		select {
		case serverErrCh <- err:
		default:
		}
	}

	// Create the test server
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Set up the "/ws" route
	app.Get("/ws", fwebsocket.New(func(c *fwebsocket.Conn) {
		defer c.Close()

		// Wrap the WebSocket connection using fiberws.Conn
		wsConnF := NewAdapter(c)

		// Initialize the server configuration
		config := wsrpc.DefaultConfig()

		// Create the WSRPC server
		wsrpcServer, err := wsrpc.NewServer(wsConnF, config)
		if err != nil {
			reportServerErr(fmt.Errorf("error creating WSRPC server: %w", err))
			return
		}
		defer wsrpcServer.Close()

		// Register the service on the server for calls from the client
		err = wsrpcServer.Register(&TestService{})
		if err != nil {
			reportServerErr(fmt.Errorf("error registering service on server: %w", err))
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = wsrpcServer.Open(ctx)
		if err != nil {
			reportServerErr(fmt.Errorf("error opening server peer: %w", err))
			return
		}

		// Server calls the Add method on the client
		args2 := &Args{A: 10, B: 15}
		reply2 := &Reply{}
		err = wsrpcServer.Call("TestService.Add", args2, reply2)
		if err != nil {
			reportServerErr(fmt.Errorf("error in RPC call from server to client: %w", err))
			return
		}

		if reply2.Sum != 25 {
			reportServerErr(fmt.Errorf("expected result 25, got %d", reply2.Sum))
			return
		}

		reportServerErr(nil)

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
			select {
			case listenerErrCh <- err:
			default:
			}
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

	// Initialize the client configuration
	config := wsrpc.DefaultConfig()

	// Create the WSRPC client
	wsrpcClient, err := wsrpc.NewClient(wsConn, config)
	if err != nil {
		t.Fatalf("Error creating client: %v", err)
	}
	defer wsrpcClient.Close()

	// Register the service on the client for calls from the server
	err = wsrpcClient.Register(&TestService{})
	if err != nil {
		t.Fatalf("Error registering service on client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = wsrpcClient.Open(ctx)
	if err != nil {
		t.Fatalf("Error opening client peer: %v", err)
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

	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server-side RPC")
	}

	select {
	case err := <-listenerErrCh:
		t.Fatalf("Fiber server failed: %v", err)
	default:
	}
}
