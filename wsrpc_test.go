package wsrpc

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

type pipeConn struct {
	net.Conn
	readMu  sync.Mutex
	writeMu sync.Mutex
	readBuf []byte
}

func newPipeConn(conn net.Conn) *pipeConn {
	return &pipeConn{Conn: conn}
}

// ReadMessage reads a message from the pipe connection.
func (p *pipeConn) ReadMessage() (messageType int, data []byte, err error) {
	p.readMu.Lock()
	defer p.readMu.Unlock()

	// Check internal buffer first
	if len(p.readBuf) > 0 {
		data = p.readBuf
		p.readBuf = nil

		return BinaryMessage, data, nil
	}

	// Read from underlying connection
	buf := make([]byte, 1024) // A small buffer just for reading I/O
	n, err := p.Conn.Read(buf)
	if err != nil {
		return -1, nil, err // Propagate errors like io.EOF
	}

	if n == 0 {
		return BinaryMessage, []byte{}, nil
	}

	data = make([]byte, n)
	copy(data, buf[:n])

	return BinaryMessage, data, nil
}

// WriteMessage writes a message to the pipe connection.
func (p *pipeConn) WriteMessage(messageType int, data []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	if messageType != BinaryMessage {
		return fmt.Errorf("pipeConn mock only supports BinaryMessage, got %d", messageType)
	}

	n, err := p.Conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write message payload: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("incomplete payload write (%d/%d bytes)", n, len(data))
	}

	return err
}

func (p *pipeConn) Close() error {
	return p.Conn.Close()
}

func (p *pipeConn) LocalAddr() net.Addr {
	return p.Conn.LocalAddr()
}

func (p *pipeConn) RemoteAddr() net.Addr {
	return p.Conn.RemoteAddr()
}

func (p *pipeConn) SetReadDeadline(t time.Time) error {
	return p.Conn.SetReadDeadline(t)
}

func (p *pipeConn) SetWriteDeadline(t time.Time) error {
	return p.Conn.SetWriteDeadline(t)
}

var _ Conn = (*pipeConn)(nil)

// --- RPC Functions ---

type Calculator struct{}

type AddArgs struct {
	A, B int
}

type AddReply struct {
	Sum int
}

func (c *Calculator) Add(args *AddArgs, reply *AddReply) error {
	reply.Sum = args.A + args.B
	return nil
}

// --- Helpers Functions ---

func setupTestPeers(t *testing.T) (client *WSRPC, server *WSRPC, clientConn *pipeConn, serverConn *pipeConn) {
	t.Helper()

	srvPipe, cliPipe := net.Pipe()
	serverConn = newPipeConn(srvPipe)
	clientConn = newPipeConn(cliPipe)

	config := DefaultConfig()
	config.LogOutput = io.Discard

	config.KeepAliveInterval = 1 * time.Second
	config.ConnectionWriteTimeout = 5 * time.Second
	config.StreamOpenTimeout = 5 * time.Second
	config.StreamCloseTimeout = 5 * time.Second

	type result struct {
		peer *WSRPC
		err  error
	}

	serverChan := make(chan result, 1)
	clientChan := make(chan result, 1)

	go func() {
		srvConfig := *config
		srv, err := NewServer(serverConn, &srvConfig)
		serverChan <- result{peer: srv, err: err}
	}()

	go func() {
		cliConfig := *config
		cli, err := NewClient(clientConn, &cliConfig)
		clientChan <- result{peer: cli, err: err}
	}()

	serverResult := <-serverChan
	clientResult := <-clientChan

	if serverResult.err != nil {
		if clientResult.peer != nil {
			clientResult.peer.Close()
		}

		serverConn.Close()
		clientConn.Close()
		t.Fatalf("Failed to create WSRPC Server: %v", serverResult.err)
	}

	if clientResult.err != nil {
		if serverResult.peer != nil {
			serverResult.peer.Close()
		}

		serverConn.Close()
		clientConn.Close()
		t.Fatalf("Failed to create WSRPC Client: %v", clientResult.err)
	}

	server = serverResult.peer
	client = clientResult.peer

	calcService := new(Calculator)

	if err := server.Register(calcService); err != nil {
		server.Close()
		client.Close()
		serverConn.Close()
		clientConn.Close()
		t.Fatalf("Failed to register service on server: %v", err)
	}

	if err := client.Register(calcService); err != nil {
		server.Close()
		client.Close()
		serverConn.Close()
		clientConn.Close()
		t.Fatalf("Failed to register service on client: %v", err)
	}

	return client, server, clientConn, serverConn
}

// --- Test Cases ---

// TestWSRPC_BasicRPC checks the basic RPC functionality of the WSRPC package.
func TestWSRPC_BasicRPC(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	client, server, clientConn, serverConn := setupTestPeers(t)
	defer client.Close()
	defer server.Close()
	defer clientConn.Close()
	defer serverConn.Close()

	// Client -> Server test
	args := &AddArgs{A: 5, B: 3}
	var reply AddReply

	callTimeout := 5 * time.Second
	errChan := make(chan error, 1)

	go func() {
		errChan <- client.Call("Calculator.Add", args, &reply)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Client -> Server RPC call failed: %v", err)
		}

		if reply.Sum != 8 {
			t.Errorf("Client -> Server RPC call expected sum %d, got %d", 8, reply.Sum)
		}
	case <-time.After(callTimeout):
		t.Fatalf("Client -> Server RPC call timed out after %v", callTimeout)
	}

	// Server -> Client test
	args = &AddArgs{A: 10, B: 20}
	reply = AddReply{}

	go func() {
		errChan <- server.Call("Calculator.Add", args, &reply)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Server -> Client RPC call failed: %v", err)
		}

		if reply.Sum != 30 {
			t.Errorf("Server -> Client RPC call expected sum %d, got %d", 30, reply.Sum)
		}
	case <-time.After(callTimeout):
		t.Fatalf("Server -> Client RPC call timed out after %v", callTimeout)
	}
}

// TestWSRPC_Close checks that closing one peer terminates the other.
func TestWSRPC_Close(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	client, server, clientConn, serverConn := setupTestPeers(t)
	defer clientConn.Close()
	defer serverConn.Close()

	closeTimeout := 5 * time.Second

	err := client.Close()
	if err != nil {
		t.Fatalf("client.Close() failed: %v", err)
	}

	select {
	case <-client.Done():
		// OK
	case <-time.After(1 * time.Second):
		t.Errorf("client.Done() did not close after client.Close()")
	}

	select {
	case <-server.Done():
		// OK
	case <-time.After(closeTimeout):
		t.Fatalf("server.Done() did not close after client closed connection within %v", closeTimeout)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Second client.Close() should not return error, got: %v", err)
	}

	err = server.Close()
	if err != nil {
		log.Printf("server.Close() returned (potentially expected) error: %v", err)
	}

	select {
	case <-server.Done():
		// OK
	case <-time.After(1 * time.Second):
		t.Errorf("server.Done() did not close within 1s after server.Close()")
	}
}

// TestWSRPC_CallAfterClose checks that calls fail after closing.
func TestWSRPC_CallAfterClose(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	client, server, clientConn, serverConn := setupTestPeers(t)
	defer server.Close()
	defer clientConn.Close()
	defer serverConn.Close()

	client.Close()

	select {
	case <-client.Done():
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("client.Done() did not close after client.Close()")
	}

	time.Sleep(100 * time.Millisecond)

	// Client -> Server test
	args := &AddArgs{A: 1, B: 1}
	var reply AddReply

	err := client.Call("Calculator.Add", args, &reply)
	if err == nil {
		t.Errorf("Expected error when calling RPC on closed client, got nil")
	} else {
		t.Logf("Got expected error calling on closed client: %v", err)

		if !errors.Is(err, rpc.ErrShutdown) && err.Error() != "rpc: client is closed" {
			t.Logf("Warning: Received error '%v', which might not be the canonical closed error", err)
		}
	}

	// Server -> Client test (after client close)
	err = server.Call("Calculator.Add", args, &reply)
	if err == nil {
		t.Errorf("Expected error when calling RPC on server with closed underlying connection, got nil")
	} else {
		t.Logf("Got expected error calling on server with closed connection: %v", err)
	}
}

func TestWSRPC_Concurrency(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	client, server, clientConn, serverConn := setupTestPeers(t)
	defer client.Close()
	defer server.Close()
	defer clientConn.Close()
	defer serverConn.Close()

	numConcurrent := 50
	callTimeout := 8 * time.Second
	var wg sync.WaitGroup
	errChan := make(chan error, numConcurrent*2)

	// Client -> Server test
	t.Logf("Starting %d concurrent Client -> Server calls", numConcurrent)
	wg.Add(numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		go func(callIdx int) {
			defer wg.Done()
			args := &AddArgs{A: callIdx, B: 1}
			var reply AddReply
			callDone := make(chan error, 1)

			go func() {
				callDone <- client.Call("Calculator.Add", args, &reply)
			}()

			select {
			case err := <-callDone:
				if err != nil {
					errChan <- fmt.Errorf("[C->S Call %d] RPC failed: %w", callIdx, err)
					return
				}

				expected := callIdx + 1

				if reply.Sum != expected {
					errChan <- fmt.Errorf("[C->S Call %d] Expected sum %d, got %d", callIdx, expected, reply.Sum)
				}
			case <-time.After(callTimeout):
				errChan <- fmt.Errorf("[C->S Call %d] RPC timed out after %v", callIdx, callTimeout)
			}
		}(i)
	}

	wg.Wait()
	t.Logf("Finished concurrent Client -> Server calls")

	// Server -> Client test
	t.Logf("Starting %d concurrent Server -> Client calls", numConcurrent)
	wg.Add(numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		go func(callIdx int) {
			defer wg.Done()
			args := &AddArgs{A: callIdx * 2, B: 5}
			var reply AddReply
			callDone := make(chan error, 1)

			go func() {
				callDone <- server.Call("Calculator.Add", args, &reply)
			}()

			select {
			case err := <-callDone:
				if err != nil {
					errChan <- fmt.Errorf("[S->C Call %d] RPC failed: %w", callIdx, err)
					return
				}

				expected := (callIdx * 2) + 5

				if reply.Sum != expected {
					errChan <- fmt.Errorf("[S->C Call %d] Expected sum %d, got %d", callIdx, expected, reply.Sum)
				}
			case <-time.After(callTimeout):
				errChan <- fmt.Errorf("[S->C Call %d] RPC timed out after %v", callIdx, callTimeout)
			}
		}(i)
	}

	wg.Wait()
	t.Logf("Finished concurrent Server -> Client calls")

	close(errChan)
	errorCount := 0
	for err := range errChan {
		t.Error(err) // Report each error found
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Concurrency test finished with %d errors", errorCount)
	} else {
		t.Log("Concurrency test finished successfully with no errors.")
	}
}
