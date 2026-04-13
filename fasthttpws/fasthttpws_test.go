package fasthttpws

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/catamat/wsrpc"
	fwebsocket "github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"
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

func TestFastHTTPWS(t *testing.T) {
	serverErrCh := make(chan error, 1)
	listenerErrCh := make(chan error, 1)
	serverReady := make(chan struct{})

	reportServerErr := func(err error) {
		select {
		case serverErrCh <- err:
		default:
		}
	}

	var upgrader = fwebsocket.FastHTTPUpgrader{
		CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
			return true
		},
	}

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			if string(ctx.Path()) != "/ws" {
				ctx.Error("not found", fasthttp.StatusNotFound)
				return
			}

			err := upgrader.Upgrade(ctx, func(wsConn *fwebsocket.Conn) {
				defer wsConn.Close()

				config := wsrpc.DefaultConfig()

				wsrpcServer, err := wsrpc.NewServer(NewAdapter(wsConn), config)
				if err != nil {
					reportServerErr(fmt.Errorf("error creating server: %w", err))
					return
				}
				defer wsrpcServer.Close()

				err = wsrpcServer.Register(&TestService{})
				if err != nil {
					reportServerErr(fmt.Errorf("error registering service on server: %w", err))
					return
				}

				openCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				err = wsrpcServer.Open(openCtx)
				if err != nil {
					reportServerErr(fmt.Errorf("error opening server peer: %w", err))
					return
				}

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

				<-wsrpcServer.Done()
			})
			if err != nil {
				reportServerErr(fmt.Errorf("error upgrading connection to WebSocket: %w", err))
			}
		},
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Error creating listener: %v", err)
	}
	defer listener.Close()

	go func() {
		close(serverReady)
		if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			select {
			case listenerErrCh <- err:
			default:
			}
		}
	}()

	<-serverReady

	time.Sleep(100 * time.Millisecond)

	wsURL := "ws://" + listener.Addr().String() + "/ws"

	wsConn, _, err := fwebsocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Error connecting to server: %v", err)
	}
	defer wsConn.Close()

	wsConnF := NewAdapter(wsConn)

	config := wsrpc.DefaultConfig()

	wsrpcClient, err := wsrpc.NewClient(wsConnF, config)
	if err != nil {
		t.Fatalf("Error creating client: %v", err)
	}
	defer wsrpcClient.Close()

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

	select {
	case err := <-listenerErrCh:
		t.Fatalf("FastHTTP server failed: %v", err)
	default:
	}
}
