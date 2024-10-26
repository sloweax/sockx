package socks5

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
)

type Dialer struct {
	address string
	network string
	config  Config
	kwargs  map[string]string
}

type Config struct {
	Methods  []Method
	Username string
	Password string
}

func NewDialer(network, address string, kwargs map[string]string, config Config) *Dialer {
	d := new(Dialer)
	d.network = network
	d.address = address
	d.config = config
	d.kwargs = kwargs
	return d
}

func (d *Dialer) KWArgs() map[string]string {
	return d.kwargs
}

func (d *Dialer) Protocol() string {
	return "socks5"
}

func (d *Dialer) String() string {
	return d.address
}

func (d *Dialer) Network() string {
	return d.network
}

func (d *Dialer) request(rw io.ReadWriter, cmd Command, addr string) error {
	address, err := NewAddress(addr)
	if err != nil {
		return err
	}

	buf := make([]byte, 3, 4+1+255+2)
	buf[0] = Version
	buf[1] = byte(cmd)
	buf[2] = 0
	buf = append(buf, address.Bytes()...)

	_, err = rw.Write(buf)

	return err
}

func (d *Dialer) response(rw io.ReadWriter) (Reply, Addr, error) {
	buf := make([]byte, 3)

	if _, err := io.ReadFull(rw, buf); err != nil {
		return 0xff, Addr{}, err
	}

	reply := Reply(buf[1])

	if buf[0] != Version {
		return reply, Addr{}, errors.New("unknown version")
	}

	if buf[2] != 0 {
		return reply, Addr{}, errors.New("invalid rsv")
	}

	bnd, err := ReadAddress(rw)
	if err != nil {
		return reply, Addr{}, err
	}

	return reply, bnd, nil
}

func (d *Dialer) negotiateMethods(rw io.ReadWriter) (Method, error) {
	if len(d.config.Methods) == 0 {
		return MethodNotAcceptable, errors.New("no methods")
	}

	if len(d.config.Methods) > math.MaxUint8 {
		return MethodNotAcceptable, errors.New("too many methods")
	}

	buf := make([]byte, 2+len(d.config.Methods))
	buf[0] = Version // ver
	buf[1] = byte(len(d.config.Methods))
	for i, m := range d.config.Methods {
		buf[i+2] = byte(m)
	}

	if _, err := rw.Write(buf); err != nil {
		return MethodNotAcceptable, err
	}

	if _, err := io.ReadFull(rw, buf[:2]); err != nil {
		return MethodNotAcceptable, err
	}

	if buf[0] != Version {
		return MethodNotAcceptable, errors.New("unknown version")
	}

	m := Method(buf[1])

	return m, nil
}

func (d *Dialer) handleAuth(rw io.ReadWriter, m Method) error {
	if !d.config.hasMethod(m) {
		return errors.New("unsupported method")
	}

	switch m {
	case MethodUserPass:
		return d.userPassAuth(rw)
	case MethodNoAuth:
		return nil
	case MethodNotAcceptable:
		return errors.New("method not acceptable")
	default:
		return errors.New("unknown method")
	}
}

func (d *Dialer) userPassAuth(rw io.ReadWriter) error {
	if len(d.config.Username) > math.MaxUint8 || len(d.config.Password) > math.MaxUint8 {
		return errors.New("username/password is too big")
	}

	buf := make([]byte, 1, 2+len(d.config.Username)+1+len(d.config.Password))
	buf[0] = VersionUserPass
	buf = append(buf, byte(len(d.config.Username)))
	buf = append(buf, []byte(d.config.Username)...)
	buf = append(buf, byte(len(d.config.Password)))
	buf = append(buf, []byte(d.config.Password)...)

	if _, err := rw.Write(buf); err != nil {
		return err
	}

	if _, err := io.ReadFull(rw, buf[:2]); err != nil {
		return err
	}

	if buf[0] != VersionUserPass {
		return errors.New("unknown username/password version")
	}
	if buf[1] != 0x00 {
		return errors.New("invalid username/password")
	}

	return nil
}

func (d *Dialer) DialContextWithConn(ctx context.Context, conn net.Conn, network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, errors.New("tcp only")
	}

	type result struct {
		err  error
		conn net.Conn
	}

	cresult := make(chan result, 1)

	go func() {
		defer close(cresult)
		method, err := d.negotiateMethods(conn)
		if err != nil {
			cresult <- result{err: err}
			return
		}

		if err = d.handleAuth(conn, method); err != nil {
			cresult <- result{err: err}
			return
		}

		if err = d.request(conn, CmdConnect, address); err != nil {
			cresult <- result{err: err}
			return
		}

		reponse, bnd, err := d.response(conn)
		if err != nil {
			cresult <- result{err: err}
			return
		}

		c := new(Conn)
		c.bnd = bnd
		c.remote, _ = NewAddress(address)
		c.conn = conn
		cresult <- result{conn: c, err: reponse.Err()}
	}()

	select {
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	case result := <-cresult:
		return result.conn, result.err
	}

}

func (c *Config) hasMethod(method Method) bool {
	for _, m := range c.Methods {
		if m == method {
			return true
		}
	}
	return false
}
