package socks5

import (
	"net"
)

type Conn struct {
	net.Conn
	// remote Addr
	bnd Addr
	// conn   net.Conn
}

func (c *Conn) BoundAddr() net.Addr {
	return &c.bnd
}
