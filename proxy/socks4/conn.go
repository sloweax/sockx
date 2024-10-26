package socks4

import (
	"net"
	"time"
)

type Conn struct {
	remote Addr
	local  net.Addr
	conn   net.Conn
}

func (c *Conn) Read(b []byte) (int, error) {
	return c.conn.Read(b)
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Write(b []byte) (int, error) {
	return c.conn.Write(b)
}

func (c *Conn) LocalAddr() net.Addr {
	return c.local
}

func (c *Conn) RemoteAddr() net.Addr {
	return &c.remote
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
