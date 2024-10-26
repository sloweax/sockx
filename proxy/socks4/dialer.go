package socks4

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

type Config struct {
	ID string
	T  int
}

type Dialer struct {
	address string
	network string
	config  Config
	kwargs  map[string]string
}

type Addr struct {
	host string
	port uint16
	t    AddressType
}

type AddressType int

const (
	Version byte = 4

	CmdConnect byte = 1

	ReplyOK       byte = 90
	ReplyRejected byte = 91

	TypeA = 1

	AtypIPv4       AddressType = 0
	AtypDomainName AddressType = 1
	AtyIPv6        AddressType = 2 // try using this on socks4a
)

func NewDialer(network, address string, kwargs map[string]string, config Config) *Dialer {
	d := new(Dialer)
	d.network = network
	d.address = address
	d.config = config
	d.kwargs = kwargs
	return d
}

func (d *Dialer) Protocol() string {
	switch d.config.T {
	case TypeA:
		return "socks4a"
	default:
		return "socks4"
	}
}

func (d *Dialer) String() string {
	return d.address
}

func (d *Dialer) KWArgs() map[string]string {
	return d.kwargs
}

func (d *Dialer) Network() string {
	return d.network
}

func (d *Dialer) request(rw io.ReadWriter, cmd byte, address string) error {
	addr, err := NewAddress(address, d.config.T)
	if err != nil {
		return err
	}

	if d.config.T == 0 && addr.t != AtypIPv4 {
		return errors.New("could not get ipv4 of hostname")
	}

	buf := make([]byte, 0, 8+len(d.config.ID)+2)
	buf = append(buf, Version, cmd)
	buf = append(buf, addr.Bytes()...)
	buf = append(buf, []byte(d.config.ID)...)
	buf = append(buf, 0)

	if _, err := rw.Write(buf); err != nil {
		return err
	}

	return nil
}

func (d *Dialer) response(rw io.ReadWriter) (byte, Addr, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(rw, buf); err != nil {
		return 0, Addr{}, err
	}

	if buf[0] != 0 {
		return buf[1], Addr{}, errors.New("unknown reply version")
	}

	addr, err := ReadAddress(rw)
	if err != nil {
		return buf[1], Addr{}, err
	}

	return buf[1], addr, nil
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
		if err := d.request(conn, CmdConnect, address); err != nil {
			cresult <- result{err: err}
			return
		}

		reply, _, err := d.response(conn)
		if err != nil {
			cresult <- result{err: err}
			return
		}

		if reply != ReplyOK {
			cresult <- result{err: fmt.Errorf("request rejected")}
			return
		}

		c := Conn{}
		c.local = conn.LocalAddr()
		c.remote, _ = NewAddress(address, d.config.T)
		c.conn = conn
		cresult <- result{conn: &c}
	}()

	select {
	case <-ctx.Done():
		conn.Close()
		return nil, ctx.Err()
	case result := <-cresult:
		return result.conn, result.err
	}
}

func NewAddress(addr string, t int) (Addr, error) {
	a := Addr{}

	host, portstr, err := net.SplitHostPort(addr)
	if err != nil {
		return Addr{}, err
	}

	port, err := strconv.ParseUint(portstr, 10, 16)
	if err != nil {
		return Addr{}, err
	}

	a.port = uint16(port)

	ip := net.ParseIP(host)
	if ip.To4() != nil {
		a.t = AtypIPv4
		a.host = ip.To4().String()
	} else if ip.To16() != nil {
		a.t = AtyIPv6
		a.host = ip.To16().String()
	} else {
		a.t = AtypDomainName
		a.host = host
	}

	if a.t == AtypDomainName && t == 0 {
		ips, err := net.LookupIP(host)
		if err != nil {
			return Addr{}, err
		}
		for _, ip := range ips {
			if ip.To4() != nil {
				a.host = ip.To4().String()
				a.t = AtypIPv4
				break
			}
		}
		if a.t != AtypIPv4 {
			return Addr{}, errors.New("could not get ipv4 of hostname")
		}
	}

	return a, nil
}

func (a *Addr) Bytes() []byte {
	buf := make([]byte, 0, net.IPv4len+2)
	buf = binary.BigEndian.AppendUint16(buf, a.port)
	switch a.t {
	case AtypIPv4:
		ip := net.ParseIP(a.host).To4()
		buf = append(buf, ip[:net.IPv4len]...)
	case AtyIPv6, AtypDomainName:
		ip := net.IPv4(0, 0, 0, 1)
		buf = append(buf, ip[:net.IPv4len]...)
		buf = append(buf, []byte(a.String())...)
		buf = append(buf, 0)
	}
	return buf
}

func ReadAddress(rw io.ReadWriter) (Addr, error) {
	a := Addr{}
	buf := make([]byte, 2+net.IPv4len)
	if _, err := io.ReadFull(rw, buf); err != nil {
		return Addr{}, err
	}
	a.port = binary.BigEndian.Uint16(buf[:2])
	ip := net.IP{}
	ip = append(ip, buf[2:]...)
	a.host = ip.To4().String()
	return a, nil
}

func (a *Addr) Network() string {
	return "tcp"
}

func (a *Addr) String() string {
	switch a.t {
	case AtyIPv6:
		return fmt.Sprintf("[%s]:%d", a.host, a.port)
	case AtypIPv4:
		return fmt.Sprintf("%s:%d", a.host, a.port)
	default:
		return fmt.Sprintf("%s:%d", a.host, a.port)
	}
}
