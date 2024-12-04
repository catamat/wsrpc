package wsrpc

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// Mock implementation of WSConn for testing
type MockWSConn struct {
	readBuffer  bytes.Buffer
	writeBuffer bytes.Buffer
}

func (c *MockWSConn) ReadMessage() (int, []byte, error) {
	if c.readBuffer.Len() == 0 {
		return 0, nil, io.EOF
	}
	data := c.readBuffer.Bytes()
	c.readBuffer.Reset()
	return 2, data, nil
}

func (c *MockWSConn) WriteMessage(messageType int, data []byte) error {
	_, err := c.writeBuffer.Write(data)
	return err
}

func (c *MockWSConn) Close() error {
	return nil
}

func (c *MockWSConn) LocalAddr() net.Addr {
	return &net.IPAddr{}
}

func (c *MockWSConn) RemoteAddr() net.Addr {
	return &net.IPAddr{}
}

func (c *MockWSConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *MockWSConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func TestWebSocketConn(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := NewWebSocketConn(mockWS)

	// Test writing
	message := []byte("hello")
	n, err := wsConn.Write(message)
	if err != nil {
		t.Fatalf("Error writing to WebSocketConn: %v", err)
	}
	if n != len(message) {
		t.Fatalf("Expected to write %d bytes, wrote %d bytes", len(message), n)
	}

	// Verify the message was written correctly
	if !bytes.Equal(mockWS.writeBuffer.Bytes(), message) {
		t.Errorf("Written message mismatch: expected %v, got %v", message, mockWS.writeBuffer.Bytes())
	}

	// Test reading
	mockWS.readBuffer.Write(message)
	readBuffer := make([]byte, len(message))
	n, err = wsConn.Read(readBuffer)
	if err != nil {
		t.Fatalf("Error reading from WebSocketConn: %v", err)
	}
	if n != len(message) {
		t.Fatalf("Expected to read %d bytes, read %d bytes", len(message), n)
	}
	if !bytes.Equal(readBuffer, message) {
		t.Errorf("Read message mismatch: expected %v, got %v", message, readBuffer)
	}
}
