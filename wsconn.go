package wsrpc

import (
	"fmt"
	"net"
	"time"
)

// The message types are defined in RFC 6455, section 11.8.
const (
	// TextMessage denotes a text data message.
	// The text message payload is interpreted as UTF-8 encoded text data.
	TextMessage = 1

	// BinaryMessage denotes a binary data message.
	BinaryMessage = 2
)

// Conn defines the interface that must be implemented by a WebSocket connection.
type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// wsConn implements net.Conn interface for WebSocket connections.
type wsConn struct {
	netConn      Conn
	readBuf      []byte
	maxChunkSize int
}

// newWSConn creates a new net.Conn instance from a WebSocket connection.
func newWSConn(conn Conn, maxChunkSize int) net.Conn {
	if maxChunkSize <= 0 {
		maxChunkSize = defaultMaxWebSocketChunkSize
	}

	return &wsConn{
		netConn:      conn,
		maxChunkSize: maxChunkSize,
	}
}

// Read reads data from the WebSocket connection.
func (c *wsConn) Read(b []byte) (n int, err error) {
	if len(c.readBuf) > 0 {
		n = copy(b, c.readBuf)
		c.readBuf = c.readBuf[n:]

		if len(c.readBuf) == 0 {
			c.readBuf = nil
		}

		return n, nil
	}

	msgType, msg, err := c.netConn.ReadMessage()
	if err != nil {
		return 0, err
	}

	if msgType != BinaryMessage {
		return 0, fmt.Errorf("expected binary message but received type %d", msgType)
	}

	n = copy(b, msg)
	if n < len(msg) {
		c.readBuf = msg[n:]
	}

	return n, nil
}

// Write writes data to the WebSocket connection.
func (c *wsConn) Write(b []byte) (n int, err error) {
	// Manually fragments data into chunks (maxMessageSize) to ensure
	// consistent behavior across different WebSocket libraries and to limit
	// memory usage for large writes, especially on slower networks.
	maxMessageSize := c.maxChunkSize

	totalWritten := 0
	remaining := len(b)

	for remaining > 0 {
		chunkSize := remaining
		if chunkSize > maxMessageSize {
			chunkSize = maxMessageSize
		}

		chunk := b[totalWritten : totalWritten+chunkSize]

		err = c.netConn.WriteMessage(BinaryMessage, chunk)
		if err != nil {
			return totalWritten, err
		}

		totalWritten += chunkSize
		remaining -= chunkSize
	}

	return totalWritten, nil
}

// Close closes the WebSocket connection.
func (c *wsConn) Close() error {
	return c.netConn.Close()
}

// LocalAddr returns the local address of the WebSocket connection.
func (c *wsConn) LocalAddr() net.Addr {
	return c.netConn.LocalAddr()
}

// RemoteAddr returns the remote address of the WebSocket connection.
func (c *wsConn) RemoteAddr() net.Addr {
	return c.netConn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines for the WebSocket connection.
func (c *wsConn) SetDeadline(t time.Time) error {
	if err := c.netConn.SetReadDeadline(t); err != nil {
		return err
	}

	return c.netConn.SetWriteDeadline(t)
}

// SetReadDeadline sets the read deadline for the WebSocket connection.
func (c *wsConn) SetReadDeadline(t time.Time) error {
	return c.netConn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline for the WebSocket connection.
func (c *wsConn) SetWriteDeadline(t time.Time) error {
	return c.netConn.SetWriteDeadline(t)
}
