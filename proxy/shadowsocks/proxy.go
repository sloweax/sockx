package shadowsocks

import (
	"context"
	"fmt"
	"net"

	"github.com/shadowsocks/go-shadowsocks2/core"
	"github.com/shadowsocks/go-shadowsocks2/socks"
)

type Dialer struct {
	cipher  core.Cipher
	kwargs  map[string]string
	network string
	address string
}

func (s *Dialer) Protocol() string {
	return "ss"
}

func (s *Dialer) KWArgs() map[string]string {
	return s.kwargs
}

func (s *Dialer) Network() string {
	return s.network
}

func (s *Dialer) String() string {
	return s.address
}

func (s *Dialer) DialContextWithConn(ctx context.Context, conn net.Conn, network, addr string) (net.Conn, error) {
	type result struct {
		net.Conn
		error
	}

	target := socks.ParseAddr(addr)
	if target == nil {
		return nil, fmt.Errorf("failed to parse address %q", addr)
	}

	c := make(chan result, 1)
	defer close(c)

	go func() {
		conn = s.cipher.StreamConn(conn)
		if _, err := conn.Write(target); err != nil {
			conn.Close()
			c <- result{error: err}
		} else {
			c <- result{Conn: conn}
		}
	}()

	select {
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	case r := <-c:
		return r.Conn, r.error
	}
}

func NewDialer(network, address string, kwargs map[string]string, method, password string) (*Dialer, error) {
	d := new(Dialer)
	d.network = network
	d.address = address
	d.kwargs = kwargs

	cipher, err := core.PickCipher(method, nil, password)
	if err != nil {
		return nil, err
	}
	d.cipher = cipher

	return d, nil
}
