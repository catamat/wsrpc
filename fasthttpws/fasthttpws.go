package fasthttpws

import (
	"net"
	"time"

	"github.com/catamat/wsrpc"

	"github.com/fasthttp/websocket"
)

// Conn implements wsnet.Conn for fasthttp/websocket.
type adapter struct {
	*websocket.Conn
}

var _ wsrpc.Conn = (*adapter)(nil)

func NewAdapter(conn *websocket.Conn) wsrpc.Conn {
	return &adapter{Conn: conn}
}

func (a *adapter) ReadMessage() (int, []byte, error) {
	return a.Conn.ReadMessage()
}

func (a *adapter) WriteMessage(messageType int, data []byte) error {
	return a.Conn.WriteMessage(messageType, data)
}

func (a *adapter) Close() error {
	return a.Conn.Close()
}

func (a *adapter) LocalAddr() net.Addr {
	return a.Conn.LocalAddr()
}

func (a *adapter) RemoteAddr() net.Addr {
	return a.Conn.RemoteAddr()
}

func (a *adapter) SetReadDeadline(t time.Time) error {
	return a.Conn.SetReadDeadline(t)
}

func (a *adapter) SetWriteDeadline(t time.Time) error {
	return a.Conn.SetWriteDeadline(t)
}
