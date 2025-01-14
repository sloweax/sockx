package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/sloweax/sockx/proxy/shadowsocks"
	"github.com/sloweax/sockx/proxy/socks4"
	"github.com/sloweax/sockx/proxy/socks5"
)

type ProxyInfo struct {
	Protocol string
	Address  string
	Args     []string
	KWArgs   map[string]string
}

type Chain []ProxyInfo

type ChainPicker interface {
	Add(Chain)
	Next() Chain
	All() []Chain
	Len() int
}

type ProxyDialer interface {
	net.Addr
	Protocol() string
	KWArgs() map[string]string
	DialContextWithConn(ctx context.Context, conn net.Conn, network, address string) (net.Conn, error)
}

func (p *ProxyInfo) ToDialer() (ProxyDialer, error) {
	switch p.Protocol {
	case "ss":
		return p.ToShadowSocks()
	case "socks5", "socks5h":
		return p.ToSOCKS5()
	case "socks4", "socks4a":
		return p.ToSOCKS4()
	default:
		return nil, fmt.Errorf("cannot convert %s to dialer", p.Protocol)
	}
}

func (p *ProxyInfo) ToSOCKS4() (ProxyDialer, error) {
	if len(p.Args) > 1 {
		return nil, fmt.Errorf("%s: invalid proxy options", p.Protocol)
	}
	config := socks4.Config{}
	if p.Protocol == "socks4a" {
		config.T = socks4.TypeA
	}
	if len(p.Args) == 1 {
		config.ID = p.Args[0]
	}
	return socks4.NewDialer("tcp", p.Address, p.KWArgs, config), nil
}

func (p *ProxyInfo) ToShadowSocks() (ProxyDialer, error) {
	network := "tcp"
	password := ""
	method := "chacha20-ietf-poly1305"
	switch len(p.Args) {
	default:
		return nil, fmt.Errorf("%s: invalid proxy options", p.Protocol)
	case 2:
		password = p.Args[1]
		fallthrough
	case 1:
		method = p.Args[0]
	case 0:
	}

	if strings.Contains(p.Address, "/") {
		network = "unix"
	}

	dialer, err := shadowsocks.NewDialer(network, p.Address, p.KWArgs, method, password)
	if err != nil {
		return nil, err
	}

	return dialer, nil
}

func (p *ProxyInfo) ToSOCKS5() (ProxyDialer, error) {
	config := socks5.Config{}
	config.Methods = append(config.Methods, socks5.MethodNoAuth)

	var network string
	if strings.Contains(p.Address, "/") {
		network = "unix"
	} else {
		network = "tcp"
	}

	switch len(p.Args) {
	case 0:
		return socks5.NewDialer(network, p.Address, p.KWArgs, config), nil
	default:
		return nil, fmt.Errorf("%s: invalid proxy options", p.Protocol)
	case 2:
		config.Password = p.Args[1]
		fallthrough
	case 1:
		config.Username = p.Args[0]
		config.Methods = append(config.Methods, socks5.MethodUserPass)
		return socks5.NewDialer(network, p.Address, p.KWArgs, config), nil
	}
}

func (p *ProxyInfo) String() string {
	a := p.Protocol
	if len(p.Address) != 0 {
		a += " " + p.Address
	}
	for _, arg := range p.Args {
		a += " " + fmt.Sprintf("%q", arg)
	}
	for k, v := range p.KWArgs {
		a += fmt.Sprintf(" %s=%q", k, v)
	}
	return a
}

func (c Chain) ToDialer() (*Dialer, error) {
	dialers := make([]ProxyDialer, len(c))

	for i, p := range c {
		d, err := p.ToDialer()
		if err != nil {
			return nil, err
		}
		dialers[i] = d
	}

	return New(dialers...), nil
}

func LoadPicker(p ChainPicker, r io.Reader) error {
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		fields, err := parseFields(line)
		if err != nil {
			return err
		}

		if len(fields) == 0 {
			continue
		}

		chain, err := parseChain(fields)
		if err != nil {
			return err
		}

		if len(chain) == 0 {
			continue
		}

		p.Add(chain)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
