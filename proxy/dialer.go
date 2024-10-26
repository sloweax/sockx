package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type Dialer struct {
	proxies []ProxyDialer
}

func New(proxies ...ProxyDialer) *Dialer {
	d := new(Dialer)
	d.proxies = proxies
	return d
}

func (d *Dialer) String() string {
	a := make([]string, 0, len(d.proxies))
	for _, p := range d.proxies {
		a = append(a, fmt.Sprintf("%s %s", p.Protocol(), p.String()))
	}
	return strings.Join(a, " | ")
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if len(d.proxies) == 0 {
		return nil, errors.New("no dialers")
	}

	p := d.proxies[0]
	entryctx, cancel, err := proxyCtx(p, ctx)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", p.Protocol(), p.String(), err)
	}
	defer cancel()

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(entryctx, p.Network(), p.String())
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", p.Protocol(), p.String(), err)
	}

	for i := 0; i < len(d.proxies); i++ {
		p := d.proxies[i]

		var (
			pnetwork string
			paddress string
			parent   context.Context
		)

		if i == len(d.proxies)-1 {
			pnetwork = network
			paddress = address
		} else {
			pnetwork = d.proxies[i+1].Network()
			paddress = d.proxies[i+1].String()
		}

		if i != 0 {
			parent = ctx
		} else {
			parent = entryctx
		}

		pctx, pcancel, err := proxyCtx(p, parent)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("%s %s: %w", p.Protocol(), p.String(), err)
		}
		defer pcancel()

		pconn, err := p.DialContextWithConn(pctx, conn, pnetwork, paddress)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("%s %s: %w", p.Protocol(), p.String(), err)
		}
		conn = pconn
	}

	p = d.proxies[len(d.proxies)-1]
	wtimeout, ok := p.KWArgs()["WriteTimeout"]
	if ok {
		err = setTimeoutStr(conn, wtimeout, conn.SetWriteDeadline)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("%s %s: %w", p.Protocol(), p.String(), err)
		}
	}

	rtimeout, ok := p.KWArgs()["ReadTimeout"]
	if ok {
		err = setTimeoutStr(conn, rtimeout, conn.SetReadDeadline)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("%s %s: %w", p.Protocol(), p.String(), err)
		}
	}

	return conn, nil
}

func setTimeoutStr(conn net.Conn, s string, fc func(time.Time) error) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	return fc(time.Now().Add(d))
}

func proxyCtx(proxy ProxyDialer, parent context.Context) (context.Context, context.CancelFunc, error) {
	durationstr, ok := proxy.KWArgs()["ConnTimeout"]
	if !ok {
		ctx, cancel := context.WithCancel(parent)
		return ctx, cancel, nil
	}

	duration, err := time.ParseDuration(durationstr)
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(parent, duration)
	return ctx, cancel, nil
}
