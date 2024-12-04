package fiberws

import (
	"net"
	"time"

	"github.com/gofiber/contrib/websocket"
)

// Conn implements wsnet.Conn for gofiber/websocket.
type Conn struct {
	*websocket.Conn
}

func (c *Conn) ReadMessage() (int, []byte, error) {
	return c.Conn.ReadMessage()
}

func (c *Conn) WriteMessage(messageType int, data []byte) error {
	return c.Conn.WriteMessage(messageType, data)
}

func (c *Conn) Close() error {
	return c.Conn.Close()
}

func (c *Conn) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}
