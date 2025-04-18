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
	ws          WSConn
	readBuf     []byte
	readMsgType int
}

// NewWebSocketConn creates a new WebSocketConn from a WSConn.
func NewWebSocketConn(ws WSConn) net.Conn {
	return &WebSocketConn{ws: ws}
}

// Read reads data from the WebSocket connection.
func (c *WebSocketConn) Read(p []byte) (n int, err error) {
	if len(c.readBuf) > 0 {
		n = copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]

		if len(c.readBuf) == 0 {
			c.readBuf = nil
		}

		return n, nil
	}

	var message []byte

	for {
		msgType, msg, err := c.ws.ReadMessage()
		if err != nil {
			return 0, err
		}

		if msgType == 2 { // BinaryMessage
			message = msg
			c.readMsgType = msgType
			break
		}
	}

	n = copy(p, message)
	if n < len(message) {
		c.readBuf = message[n:]
	}

	return n, nil
}

// Write writes data to the WebSocket connection.
func (c *WebSocketConn) Write(b []byte) (n int, err error) {
	const maxMessageSize = 8192

	totalWritten := 0
	remaining := len(b)

	for remaining > 0 {
		chunkSize := remaining
		if chunkSize > maxMessageSize {
			chunkSize = maxMessageSize
		}

		chunk := b[totalWritten : totalWritten+chunkSize]

		err = c.ws.WriteMessage(2, chunk) // BinaryMessage
		if err != nil {
			return totalWritten, err
		}

		totalWritten += chunkSize
		remaining -= chunkSize
	}

	return totalWritten, nil
}

// Close closes the WebSocket connection.
func (c *WebSocketConn) Close() error {
	return c.ws.Close()
}

// LocalAddr returns the local network address.
func (c *WebSocketConn) LocalAddr() net.Addr {
	return c.ws.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *WebSocketConn) RemoteAddr() net.Addr {
	return c.ws.RemoteAddr()
}

// SetDeadline sets the read and write deadlines for the WebSocket connection.
func (c *WebSocketConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}

// SetReadDeadline sets the read deadline for the WebSocket connection.
func (c *WebSocketConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline for the WebSocket connection.
func (c *WebSocketConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}
