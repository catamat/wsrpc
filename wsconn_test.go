package wsrpc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// Mock implementation of wsrpc.Conn for testing wsConn adapter.
type MockWSConn struct {
	readBuffer  [][]byte
	readTypes   []int
	readError   error
	writeBuffer bytes.Buffer
	writeError  error
	closed      bool
}

// AddMessage adds a message to be read by ReadMessage.
func (c *MockWSConn) AddMessage(msgType int, data []byte) {
	c.readTypes = append(c.readTypes, msgType)
	c.readBuffer = append(c.readBuffer, data)
}

// SetReadError sets an error to be returned by the next ReadMessage call.
func (c *MockWSConn) SetReadError(err error) {
	c.readError = err
}

// SetWriteError sets an error to be returned by the next WriteMessage call.
func (c *MockWSConn) SetWriteError(err error) {
	c.writeError = err
}

// ReadMessage simulates reading a WebSocket message.
func (c *MockWSConn) ReadMessage() (int, []byte, error) {
	if c.closed {
		return 0, nil, io.ErrClosedPipe
	}

	if c.readError != nil {
		err := c.readError
		c.readError = nil
		return 0, nil, err
	}

	if len(c.readBuffer) == 0 {
		return 0, nil, io.EOF
	}

	msgType := c.readTypes[0]
	data := c.readBuffer[0]
	c.readTypes = c.readTypes[1:]
	c.readBuffer = c.readBuffer[1:]

	return msgType, data, nil
}

// WriteMessage simulates writing a WebSocket message.
func (c *MockWSConn) WriteMessage(messageType int, data []byte) error {
	if c.closed {
		return io.ErrClosedPipe
	}

	if c.writeError != nil {
		err := c.writeError
		c.writeError = nil
		return err
	}

	if messageType != BinaryMessage {
		return fmt.Errorf("mock: expected BinaryMessage type, got %d", messageType)
	}
	_, err := c.writeBuffer.Write(data)
	return err
}

// Close simulates closing the connection.
func (c *MockWSConn) Close() error {
	if c.closed {
		return io.ErrClosedPipe
	}
	c.closed = true
	return nil
}

func (c *MockWSConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (c *MockWSConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
}

func (c *MockWSConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *MockWSConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// --- Test Cases ---

func TestWSConn_Read_Simple(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	expectedMessage := []byte("hello world")
	mockWS.AddMessage(BinaryMessage, expectedMessage)

	readBuffer := make([]byte, len(expectedMessage))
	n, err := wsConn.Read(readBuffer)

	if err != nil {
		t.Fatalf("Read failed unexpectedly: %v", err)
	}

	if n != len(expectedMessage) {
		t.Fatalf("Expected to read %d bytes, but got %d", len(expectedMessage), n)
	}

	if !bytes.Equal(readBuffer, expectedMessage) {
		t.Errorf("Read message mismatch: expected %q, got %q", expectedMessage, readBuffer)
	}
}

func TestWSConn_Read_SmallBuffer(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	fullMessage := []byte("this is a longer message")
	mockWS.AddMessage(BinaryMessage, fullMessage)

	bufferSize := 10
	readBuffer1 := make([]byte, bufferSize)

	n1, err1 := wsConn.Read(readBuffer1)

	if err1 != nil {
		t.Fatalf("First Read failed unexpectedly: %v", err1)
	}

	if n1 != bufferSize {
		t.Fatalf("First Read: Expected to read %d bytes, but got %d", bufferSize, n1)
	}

	if !bytes.Equal(readBuffer1, fullMessage[:bufferSize]) {
		t.Errorf("First Read mismatch: expected %q, got %q", fullMessage[:bufferSize], readBuffer1)
	}

	expectedInternal := fullMessage[bufferSize:]
	readBuffer2 := make([]byte, len(expectedInternal)+5)
	n2, err2 := wsConn.Read(readBuffer2)

	if err2 != nil {
		t.Fatalf("Second Read failed unexpectedly: %v", err2)
	}

	if n2 != len(expectedInternal) {
		t.Fatalf("Second Read: Expected to read %d bytes, but got %d", len(expectedInternal), n2)
	}

	if !bytes.Equal(readBuffer2[:n2], expectedInternal) {
		t.Errorf("Second Read mismatch: expected %q, got %q", expectedInternal, readBuffer2[:n2])
	}
}

func TestWSConn_Read_NonBinaryMessage(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	mockWS.AddMessage(TextMessage, []byte("this is text"))

	readBuffer := make([]byte, 20)
	_, err := wsConn.Read(readBuffer)

	if err == nil {
		t.Fatalf("Expected an error when reading a non-binary message, but got nil")
	}

	expectedErrorMsg := fmt.Sprintf("expected binary message but received type %d", TextMessage)

	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error %q, but got %q", expectedErrorMsg, err.Error())
	}
}

func TestWSConn_Read_ReadMessageError(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	expectedError := errors.New("simulated network error")
	mockWS.SetReadError(expectedError)

	readBuffer := make([]byte, 10)
	_, err := wsConn.Read(readBuffer)

	if !errors.Is(err, expectedError) {
		t.Fatalf("Expected error %v, but got %v", expectedError, err)
	}
}

func TestWSConn_Write_Simple(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	message := []byte("write this")
	n, err := wsConn.Write(message)

	if err != nil {
		t.Fatalf("Write failed unexpectedly: %v", err)
	}

	if n != len(message) {
		t.Fatalf("Expected Write to return %d, but got %d", len(message), n)
	}

	writtenData := mockWS.writeBuffer.Bytes()

	if !bytes.Equal(writtenData, message) {
		t.Errorf("Data written to mock mismatch: expected %q, got %q", message, writtenData)
	}
}

func TestWSConn_Write_Chunking(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	largeMessageSize := 10000
	largeMessage := bytes.Repeat([]byte{'a'}, largeMessageSize)

	n, err := wsConn.Write(largeMessage)

	if err != nil {
		t.Fatalf("Write failed unexpectedly: %v", err)
	}

	if n != largeMessageSize {
		t.Fatalf("Expected Write to return %d, but got %d", largeMessageSize, n)
	}

	writtenData := mockWS.writeBuffer.Bytes()

	if !bytes.Equal(writtenData, largeMessage) {
		t.Errorf("Data written to mock mismatch after chunking")
	}
}

func TestWSConn_Write_WriteMessageError(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	expectedError := errors.New("simulated write failure")
	mockWS.SetWriteError(expectedError)

	message := []byte("try to write this")
	n, err := wsConn.Write(message)

	if !errors.Is(err, expectedError) {
		t.Fatalf("Expected error %v, but got %v", expectedError, err)
	}

	if n != 0 {
		t.Errorf("Expected bytes written to be 0 on first chunk error, but got %d", n)
	}
}

func TestWSConn_Close(t *testing.T) {
	mockWS := &MockWSConn{}
	wsConn := newWSConn(mockWS, 0)

	if err := wsConn.Close(); err != nil {
		t.Fatalf("Close failed unexpectedly: %v", err)
	}

	if !mockWS.closed {
		t.Errorf("MockWSConn was not marked as closed")
	}
}
