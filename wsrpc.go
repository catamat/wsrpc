package wsrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/rpc"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

const defaultMaxWebSocketChunkSize = 8192
const internalRPCServiceName = "_WSRPC"
const internalRPCOpenMethod = internalRPCServiceName + ".Open"

var (
	// ErrNotOpen indicates that the peer has not completed the readiness handshake yet.
	ErrNotOpen = errors.New("wsrpc: peer is not open")

	// ErrAlreadyOpen indicates that registration is no longer allowed because opening has started.
	ErrAlreadyOpen = errors.New("wsrpc: peer is already opening or open")

	// ErrClosed indicates that the connection has already been closed.
	ErrClosed = errors.New("wsrpc: connection is closed")
)

// WSRPC defines a bidirectional RPC connection over a Yamux session.
type WSRPC struct {
	mu        sync.Mutex
	openMu    sync.Mutex
	session   *yamux.Session
	rpcServer *rpc.Server
	rpcClient *rpc.Client
	closeChan chan struct{}
	doneChan  chan struct{}
	readyChan chan struct{}
	open      bool
	opening   bool
	logOutput io.Writer
}

type controlService struct {
	peer *WSRPC
}

func (c *controlService) Open(_ *int, _ *int) error {
	select {
	case <-c.peer.readyChan:
		return nil
	case <-c.peer.closeChan:
		return ErrClosed
	}
}

// Config defines the configuration options.
type Config struct {
	AcceptBacklog          int
	EnableKeepAlive        bool
	KeepAliveInterval      time.Duration
	ConnectionWriteTimeout time.Duration
	MaxStreamWindowSize    uint32
	StreamOpenTimeout      time.Duration
	StreamCloseTimeout     time.Duration
	LogOutput              io.Writer
	MaxWebSocketChunkSize  int
}

// DefaultConfig is used to return a default configuration.
func DefaultConfig() *Config {
	return &Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      30 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    256 * 1024,
		StreamCloseTimeout:     5 * time.Minute,
		StreamOpenTimeout:      75 * time.Second,
		LogOutput:              os.Stderr,
		MaxWebSocketChunkSize:  defaultMaxWebSocketChunkSize,
	}
}

// NewServer creates a new server-side peer on a WebSocket-backed connection.
// Register any local services and call Open before using Call.
func NewServer(conn Conn, config *Config) (*WSRPC, error) {
	if config == nil {
		config = DefaultConfig()
	}

	wsConn := newWSConn(conn, config.MaxWebSocketChunkSize)

	yamuxConfig := &yamux.Config{
		AcceptBacklog:          config.AcceptBacklog,
		EnableKeepAlive:        config.EnableKeepAlive,
		KeepAliveInterval:      config.KeepAliveInterval,
		ConnectionWriteTimeout: config.ConnectionWriteTimeout,
		MaxStreamWindowSize:    config.MaxStreamWindowSize,
		StreamCloseTimeout:     config.StreamCloseTimeout,
		StreamOpenTimeout:      config.StreamOpenTimeout,
		LogOutput:              config.LogOutput,
	}

	session, err := yamux.Server(wsConn, yamuxConfig)
	if err != nil {
		return nil, err
	}

	wsrpc, err := createPeer(session, config.LogOutput)
	if err != nil {
		session.Close()
		return nil, err
	}

	return wsrpc, nil
}

// NewClient creates a new client-side peer on a WebSocket-backed connection.
// Register any local services and call Open before using Call.
func NewClient(conn Conn, config *Config) (*WSRPC, error) {
	if config == nil {
		config = DefaultConfig()
	}

	wsConn := newWSConn(conn, config.MaxWebSocketChunkSize)

	yamuxConfig := &yamux.Config{
		AcceptBacklog:          config.AcceptBacklog,
		EnableKeepAlive:        config.EnableKeepAlive,
		KeepAliveInterval:      config.KeepAliveInterval,
		ConnectionWriteTimeout: config.ConnectionWriteTimeout,
		MaxStreamWindowSize:    config.MaxStreamWindowSize,
		StreamCloseTimeout:     config.StreamCloseTimeout,
		StreamOpenTimeout:      config.StreamOpenTimeout,
		LogOutput:              config.LogOutput,
	}

	session, err := yamux.Client(wsConn, yamuxConfig)
	if err != nil {
		return nil, err
	}

	wsrpc, err := createPeer(session, config.LogOutput)
	if err != nil {
		session.Close()
		return nil, err
	}

	return wsrpc, nil
}

// createPeer creates a new RPC peer.
func createPeer(session *yamux.Session, logOutput io.Writer) (*WSRPC, error) {
	wsrpc := &WSRPC{
		session:   session,
		rpcServer: rpc.NewServer(),
		closeChan: make(chan struct{}),
		doneChan:  make(chan struct{}),
		readyChan: make(chan struct{}),
		logOutput: logOutput,
	}

	if err := wsrpc.rpcServer.RegisterName(internalRPCServiceName, &controlService{peer: wsrpc}); err != nil {
		return nil, err
	}

	// Start accepting incoming streams for the RPC service
	go wsrpc.acceptStreams()

	return wsrpc, nil
}

// acceptStreams accepts incoming streams for the RPC service.
func (wsrpc *WSRPC) acceptStreams() {
	for {
		stream, err := wsrpc.session.Accept()
		if err != nil {
			// Check if the session was closed
			select {
			case <-wsrpc.closeChan:
				return
			default:
				if wsrpc.logOutput != io.Discard && wsrpc.logOutput != nil {
					fmt.Fprintf(wsrpc.logOutput, "error accepting yamux stream: %v. closing session.\n", err)
				}

				wsrpc.Close()
				return
			}

		}

		go wsrpc.rpcServer.ServeConn(stream)
	}
}

// Register registers the set of methods exposed to the remote peer.
// It must be called before Open.
func (wsrpc *WSRPC) Register(rcvr interface{}) error {
	wsrpc.mu.Lock()
	defer wsrpc.mu.Unlock()

	select {
	case <-wsrpc.closeChan:
		return ErrClosed
	default:
	}

	if wsrpc.opening || wsrpc.open {
		return ErrAlreadyOpen
	}

	return wsrpc.rpcServer.Register(rcvr)
}

// Open completes the readiness handshake with the remote peer and enables outbound RPC calls.
// Local services become visible to the remote peer as soon as Open starts waiting.
func (wsrpc *WSRPC) Open(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	wsrpc.openMu.Lock()
	defer wsrpc.openMu.Unlock()

	wsrpc.mu.Lock()
	select {
	case <-wsrpc.closeChan:
		wsrpc.mu.Unlock()
		return ErrClosed
	default:
	}

	if wsrpc.open {
		wsrpc.mu.Unlock()
		return nil
	}

	if !wsrpc.opening {
		wsrpc.opening = true
		close(wsrpc.readyChan)
	}
	wsrpc.mu.Unlock()

	stream, err := wsrpc.session.Open()
	if err != nil {
		return err
	}

	client := rpc.NewClient(stream)
	arg := 0
	reply := 0
	call := client.Go(internalRPCOpenMethod, &arg, &reply, make(chan *rpc.Call, 1))

	select {
	case result := <-call.Done:
		if result.Error != nil {
			client.Close()
			return result.Error
		}
	case <-ctx.Done():
		client.Close()
		return ctx.Err()
	case <-wsrpc.closeChan:
		client.Close()
		return ErrClosed
	}

	wsrpc.mu.Lock()
	defer wsrpc.mu.Unlock()

	select {
	case <-wsrpc.closeChan:
		client.Close()
		return ErrClosed
	default:
	}

	if wsrpc.open {
		client.Close()
		return nil
	}

	wsrpc.rpcClient = client
	wsrpc.open = true

	return nil
}

// Call calls the named function on the remote peer.
// Open must complete successfully before Call can be used.
func (wsrpc *WSRPC) Call(serviceMethod string, args interface{}, reply interface{}) error {
	wsrpc.mu.Lock()
	client := wsrpc.rpcClient
	open := wsrpc.open
	wsrpc.mu.Unlock()

	if !open || client == nil {
		select {
		case <-wsrpc.closeChan:
			return ErrClosed
		default:
			return ErrNotOpen
		}
	}

	return client.Call(serviceMethod, args, reply)
}

// Close closes the connection.
func (wsrpc *WSRPC) Close() error {
	wsrpc.mu.Lock()

	select {
	case <-wsrpc.closeChan:
		wsrpc.mu.Unlock()
		// Already closed
		return nil
	default:
		close(wsrpc.closeChan)
	}

	client := wsrpc.rpcClient
	wsrpc.rpcClient = nil
	wsrpc.open = false
	wsrpc.mu.Unlock()

	defer close(wsrpc.doneChan)

	if client != nil {
		client.Close()
	}

	return wsrpc.session.Close()
}

// Done returns a channel that is closed when the connection is terminated.
func (wsrpc *WSRPC) Done() <-chan struct{} {
	return wsrpc.doneChan
}
