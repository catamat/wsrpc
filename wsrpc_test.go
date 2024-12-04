package wsrpc

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/goleak"
	"golang.org/x/exp/rand"
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

func TestWSRPC(t *testing.T) {
	defer goleak.VerifyNone(t)

	var upgrader = websocket.Upgrader{
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
			t.Fatalf("Error upgrading connection to WebSocket: %v", err)
		}
		defer wsConn.Close()

		// Create server connection
		serverConn := NewWebSocketConn(wsConn)

		// Create the WSRPC server
		wsrpcServer, err := NewServer(serverConn)
		if err != nil {
			t.Fatalf("Error creating server: %v", err)
		}
		defer wsrpcServer.Close()

		// Register the service on the server for calls from the client
		err = wsrpcServer.Register(&TestService{})
		if err != nil {
			t.Fatalf("Error registering service on server: %v", err)
		}

		// Server calls the Add method on the client
		args2 := &Args{A: 10, B: 15}
		reply2 := &Reply{}
		err = wsrpcServer.Call("TestService.Add", args2, reply2)
		if err != nil {
			t.Fatalf("Error in RPC call from server to client: %v", err)
		}

		if reply2.Sum != 25 {
			t.Errorf("Expected result 25, got %d", reply2.Sum)
		}

		// Wait until the connection is closed
		<-wsrpcServer.Done()
	}))
	defer server.Close()

	// Convert the server's URL to a WebSocket URL and append "/ws"
	wsURL := "ws" + server.URL[len("http"):] + "/ws"

	// Create the client
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Error connecting to server: %v", err)
	}
	defer wsConn.Close()

	// Create client connection
	clientConn := NewWebSocketConn(wsConn)

	// Create the WSRPC client
	wsrpcClient, err := NewClient(clientConn)
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
		t.Fatalf("Error in RPC call: %v", err)
	}

	if reply.Sum != 12 {
		t.Errorf("Expected result 12, got %d", reply.Sum)
	}
}

func BenchmarkWSRPC(b *testing.B) {
	defer goleak.VerifyNone(b)

	// Number of clients to simulate.
	numClients := 100

	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Create the test server with a WebSocket handler.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Upgrade the HTTP connection to a WebSocket connection.
		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Create server connection.
		serverConn := NewWebSocketConn(wsConn)

		// Create the WSRPC server.
		wsrpcServer, err := NewServer(serverConn)
		if err != nil {
			return
		}

		// Register the service on the server for calls from the client.
		err = wsrpcServer.Register(&TestService{})
		if err != nil {
			return
		}

		// Wait until the connection is closed.
		<-wsrpcServer.Done()
	}))
	defer server.Close()

	// Convert the server's URL to a WebSocket URL and append "/ws".
	wsURL := "ws" + server.URL[len("http"):] + "/ws"

	// Start the clients.
	clients := make([]*WSRPC, numClients)
	for i := 0; i < numClients; i++ {
		// Create the client.
		wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			b.Fatalf("Error connecting to server: %v", err)
		}

		// Create client connection.
		clientConn := NewWebSocketConn(wsConn)

		// Create the WSRPC client.
		wsrpcClient, err := NewClient(clientConn)
		if err != nil {
			b.Fatalf("Error creating client: %v", err)
		}

		// Register the service on the client for calls from the server.
		err = wsrpcClient.Register(&TestService{})
		if err != nil {
			b.Fatalf("Error registering service on client: %v", err)
		}

		clients[i] = wsrpcClient
	}

	// Ensure all clients are closed after the benchmark.
	defer func() {
		for _, client := range clients {
			client.Close()
		}
	}()

	// Prepare arguments.
	args := &Args{A: 5, B: 7}

	// Reset the timer to exclude setup time from the benchmark.
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		// Use a thread-local random source to select clients.
		rnd := rand.New(rand.NewSource(uint64(time.Now().UnixNano())))
		reply := &Reply{}

		for pb.Next() {
			client := clients[rnd.Intn(numClients)]

			err := client.Call("TestService.Add", args, reply)
			if err != nil {
				b.Error(err)
				return
			}
			if reply.Sum != 12 {
				b.Errorf("Expected result 12, got %d", reply.Sum)
				return
			}
		}
	})
}
