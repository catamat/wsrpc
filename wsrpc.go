package wsrpc

import (
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

// WSRPC defines a bidirectional RPC connection over a Yamux session.
type WSRPC struct {
	mu        sync.Mutex
	session   *yamux.Session
	rpcServer *rpc.Server
	rpcClient *rpc.Client
	closeChan chan struct{}
	logOutput io.Writer
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

// NewServer creates a new server on a net.Conn connection.
func NewServer(conn Conn, config *Config) (*WSRPC, error) {
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

// NewClient creates a new client on a net.Conn connection.
func NewClient(conn Conn, config *Config) (*WSRPC, error) {
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
		logOutput: logOutput,
	}

	// Start accepting incoming streams for the RPC service
	go wsrpc.acceptStreams()

	// Open a stream for the RPC client
	stream, err := session.Open()
	if err != nil {
		return nil, err
	}

	wsrpc.rpcClient = rpc.NewClient(stream)

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

// Register registers the set of methods of the RPC service.
func (wsrpc *WSRPC) Register(rcvr interface{}) error {
	return wsrpc.rpcServer.Register(rcvr)
}

// Call calls the named function, waits for it to complete, and returns its error status.
func (wsrpc *WSRPC) Call(serviceMethod string, args interface{}, reply interface{}) error {
	if wsrpc.rpcClient == nil {
		return errors.New("rpc client is not initialized")
	}
	return wsrpc.rpcClient.Call(serviceMethod, args, reply)
}

// Close closes the connection.
func (wsrpc *WSRPC) Close() error {
	wsrpc.mu.Lock()
	defer wsrpc.mu.Unlock()

	select {
	case <-wsrpc.closeChan:
		// Already closed
		return nil
	default:
		close(wsrpc.closeChan)
	}

	if wsrpc.rpcClient != nil {
		wsrpc.rpcClient.Close()
	}

	return wsrpc.session.Close()
}

// Done returns a channel that is closed when the connection is terminated.
func (wsrpc *WSRPC) Done() <-chan struct{} {
	return wsrpc.closeChan
}
