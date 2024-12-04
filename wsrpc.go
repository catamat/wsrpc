package wsrpc

import (
	"errors"
	"io"
	"net"
	"net/rpc"
	"sync"

	"github.com/hashicorp/yamux"
)

// WSRPC defines a bidirectional RPC connection over a Yamux session.
type WSRPC struct {
	mu        sync.Mutex
	session   *yamux.Session
	rpcServer *rpc.Server
	rpcClient *rpc.Client
	closeChan chan struct{}
}

// NewServer creates a new server on a net.Conn connection.
func NewServer(conn net.Conn) (*WSRPC, error) {
	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.LogOutput = io.Discard

	session, err := yamux.Server(conn, yamuxConfig)
	if err != nil {
		return nil, err
	}

	wsrpc, err := createPeer(session)
	if err != nil {
		session.Close()
		return nil, err
	}

	return wsrpc, nil
}

// NewClient creates a new client on a net.Conn connection.
func NewClient(conn net.Conn) (*WSRPC, error) {
	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.LogOutput = io.Discard

	session, err := yamux.Client(conn, yamuxConfig)
	if err != nil {
		return nil, err
	}

	wsrpc, err := createPeer(session)
	if err != nil {
		session.Close()
		return nil, err
	}

	return wsrpc, nil
}

// createPeer creates a new RPC peer.
func createPeer(session *yamux.Session) (*WSRPC, error) {
	wsrpc := &WSRPC{
		session:   session,
		rpcServer: rpc.NewServer(),
		closeChan: make(chan struct{}),
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
			}

			wsrpc.Close()
			return
		}

		wsrpc.rpcServer.ServeConn(stream)
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
