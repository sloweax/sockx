package socks5

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

const (
	CmdConnect Command = 0x01

	AtypIPV4       AddressType = 0x01
	AtypDomainName AddressType = 0x03
	AtypIPV6       AddressType = 0x04

	MethodNoAuth        Method = 0x00
	MethodUserPass      Method = 0x02
	MethodNotAcceptable Method = 0xFF

	Version         byte = 0x05
	VersionUserPass byte = 0x01

	ReplyOK                 Reply = 0x00
	ReplyGeneralFailure     Reply = 0x01
	ReplyConnNotAllowed     Reply = 0x02
	ReplyNetworkUnreachable Reply = 0x03
	ReplyHostUnreachable    Reply = 0x04
	ReplyConnRefused        Reply = 0x05
	ReplyTTLExpired         Reply = 0x06
	ReplyCmdNotSupported    Reply = 0x07
	ReplyAtypNotSupported   Reply = 0x08
)

type Command byte
type AddressType byte
type Method byte
type Reply byte

type Addr struct {
	addr string
	port uint16
	atyp AddressType
}

func NewAddress(address string) (Addr, error) {
	host, portstr, err := net.SplitHostPort(address)
	if err != nil {
		return Addr{}, err
	}
	a := Addr{}
	a.addr = host
	port, err := strconv.ParseUint(portstr, 10, 16)
	if err != nil {
		return Addr{}, err
	}
	a.port = uint16(port)

	ip := net.ParseIP(host)
	if ip == nil {
		a.atyp = AtypDomainName
		a.addr = host
		if len(a.addr) > 255 {
			return Addr{}, errors.New("socks5: hostname length is too big")
		}
	} else if ip.To4() != nil {
		a.atyp = AtypIPV4
		a.addr = host
	} else {
		a.atyp = AtypIPV6
		a.addr = host
	}

	return a, nil
}

func ReadAddress(r io.Reader) (Addr, error) {
	buf := make([]byte, 1+255+2)
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return Addr{}, err
	}
	a := Addr{}
	a.atyp = AddressType(buf[0])
	switch a.atyp {
	case AtypIPV4:
		if _, err := io.ReadFull(r, buf[:net.IPv4len]); err != nil {
			return Addr{}, err
		}
		ip := net.IPv4(buf[0], buf[1], buf[2], buf[3])
		a.addr = ip.String()
	case AtypIPV6:
		ip := net.IP{}
		if _, err := io.ReadFull(r, buf[:net.IPv6len]); err != nil {
			return Addr{}, err
		}
		ip = append(ip, buf[:net.IPv6len]...)
		a.addr = ip.String()
	case AtypDomainName:
		if _, err := io.ReadFull(r, buf[:1]); err != nil {
			return Addr{}, err
		}
		len := int(buf[0])
		if _, err := io.ReadFull(r, buf[:len]); err != nil {
			return Addr{}, err
		}
		a.addr = string(buf[:len])
	default:
		return Addr{}, errors.New("socks5: invalid address type")
	}

	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return Addr{}, err
	}

	a.port = binary.BigEndian.Uint16(buf[:2])

	return a, nil
}

func (a *Addr) String() string {
	addr := a.addr
	if a.atyp == AtypIPV6 {
		addr = fmt.Sprintf("[%s]", addr)
	}
	return fmt.Sprintf("%s:%d", addr, a.port)
}

func (a *Addr) Network() string {
	return "tcp"
}

func (a *Addr) Bytes() []byte {
	buf := make([]byte, 1, 1+255+2)
	buf[0] = byte(a.atyp)
	switch a.atyp {
	case AtypDomainName:
		buf = append(buf, byte(len(a.addr)))
		buf = append(buf, []byte(a.addr)...)
	case AtypIPV6:
		ip := net.ParseIP(a.addr)
		buf = append(buf, ip.To16()...)
	case AtypIPV4:
		ip := net.ParseIP(a.addr)
		buf = append(buf, ip.To4()[:4]...)
	}
	buf = binary.BigEndian.AppendUint16(buf, a.port)
	return buf
}

func (m Method) isValid() bool {
	switch m {
	case MethodNoAuth, MethodNotAcceptable:
		return true
	default:
		return false
	}
}

func (r Reply) Err() error {
	switch r {
	case ReplyTTLExpired:
		return errors.New("TTL expired")
	case ReplyNetworkUnreachable:
		return errors.New("network unreachable")
	case ReplyHostUnreachable:
		return errors.New("host unreachable")
	case ReplyGeneralFailure:
		return errors.New("general failure")
	case ReplyConnRefused:
		return errors.New("connection refused")
	case ReplyConnNotAllowed:
		return errors.New("connection not allowed")
	case ReplyCmdNotSupported:
		return errors.New("command not supported")
	case ReplyAtypNotSupported:
		return errors.New("address type not supported")
	case ReplyOK:
		return nil
	default:
		return errors.New("unknown reply")
	}
}
