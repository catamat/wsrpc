package wsrpc

import (
	"net"
	"time"
)

// WSConn defines the interface that must be implemented by a WebSocket connection.
type WSConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// WebSocketConn adapts a WSConn to a net.Conn.
type WebSocketConn struct {
	ws WSConn
}

// NewWebSocketConn creates a new WebSocketConn from a WSConn.
func NewWebSocketConn(ws WSConn) net.Conn {
	return &WebSocketConn{ws: ws}
}

func (c *WebSocketConn) Read(b []byte) (n int, err error) {
	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			return 0, err
		}
		if len(message) > 0 {
			return copy(b, message), nil
		}
	}
}

func (c *WebSocketConn) Write(b []byte) (n int, err error) {
	err = c.ws.WriteMessage(2, b) // 2 = BinaryMessage
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *WebSocketConn) Close() error {
	return c.ws.Close()
}

func (c *WebSocketConn) LocalAddr() net.Addr {
	return c.ws.LocalAddr()
}

func (c *WebSocketConn) RemoteAddr() net.Addr {
	return c.ws.RemoteAddr()
}

func (c *WebSocketConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}

func (c *WebSocketConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

func (c *WebSocketConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}
