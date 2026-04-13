package gorillaws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/catamat/wsrpc"
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

func TestGorillaWS(t *testing.T) {
	serverErrCh := make(chan error, 1)

	reportServerErr := func(err error) {
		select {
		case serverErrCh <- err:
		default:
		}
	}

	var upgrader = gwebsocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Create the test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the path matches "/ws"
		if r.URL.Path != "/ws" {
			http.NotFound(w, r)
			return
		}

		// Upgrade the HTTP connection to a WebSocket connection
		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			reportServerErr(fmt.Errorf("error upgrading connection to WebSocket: %w", err))
			return
		}
		defer wsConn.Close()

		// Wrap the WebSocket connection using gorillaws.Conn
		wsConnG := NewAdapter(wsConn)

		// Initialize the server configuration
		config := wsrpc.DefaultConfig()

		// Create the WSRPC server
		wsrpcServer, err := wsrpc.NewServer(wsConnG, config)
		if err != nil {
			reportServerErr(fmt.Errorf("error creating server: %w", err))
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
	defer server.Close()

	// Give the server some time to start
	time.Sleep(100 * time.Millisecond)

	// Convert the server's URL to a WebSocket URL and append "/ws"
	wsURL := "ws" + server.URL[len("http"):] + "/ws"

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
		t.Fatalf("Error in RPC call: %v", err)
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
}
