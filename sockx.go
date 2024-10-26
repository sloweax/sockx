package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sloweax/sockx/proxy"
	"github.com/sloweax/sockx/proxy/socks5"
)

type StringArray []string

func main() {

	var (
		proxy_files StringArray
		addr        string
		verbose     bool
		retry       int
		network     string
	)

	flag.Var(&proxy_files, "c", "load config file")
	flag.StringVar(&addr, "a", "127.0.0.1:1080", "listen on address")
	flag.IntVar(&retry, "r", 0, "if chain connection fails, retry with another one x times")
	flag.StringVar(&network, "n", "tcp", "listen on network (tcp,unix)")
	flag.BoolVar(&verbose, "verbose", false, "log additional info")
	flag.Parse()

	picker := proxy.RoundRobin{}

	for _, file := range proxy_files {
		f, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}

		if err := picker.Load(f); err != nil {
			log.Fatal(err)
		}

		f.Close()
	}

	if len(proxy_files) == 0 {
		log.Print("no specified config files, reading from stdin")
		err := picker.Load(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
	}

	if picker.Len() == 0 {
		log.Fatal("no loaded proxies")
	}

	if verbose {
		for i, ps := range picker.All() {
			chain := make([]string, len(ps))
			for i, p := range ps {
				chain[i] = p.String()
			}
			log.Printf("chain %d: %s", i, strings.Join(chain, " | "))
		}
	}

	server := socks5.NewServer()
	err := server.Listen(network, addr)
	if err != nil {
		log.Fatal(err)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Kill, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigc
		server.Close()
	}()

	for {
		conn, err := server.Accept()
		if err != nil {
			if server.Closed() {
				break
			}
			log.Print(fmt.Errorf("server: %w", err))
			continue
		}

		go func() {
			defer conn.Close()

			var (
				err   error
				rconn net.Conn
				proxy *proxy.Dialer
			)

			raddr, err := server.Handle(conn)
			if err != nil {
				log.Print(fmt.Errorf("server: %w", err))
				return
			}

			for i := 0; i < retry+1; i++ {
				var (
					ctx    context.Context
					cancel context.CancelFunc
				)

				chain := picker.Next()
				proxy, err = chain.ToDialer()
				if err != nil {
					log.Print(fmt.Errorf("server: %w", err))
					return
				}

				timeoutstr, ok := chain[0].KWArgs["ChainConnTimeout"]
				if ok {
					duration, err := time.ParseDuration(timeoutstr)
					if err != nil {
						log.Print(err)
						return
					}
					ctx, cancel = context.WithTimeout(context.Background(), duration)
					defer cancel()
				} else {
					ctx = context.Background()
				}

				rconn, err = proxy.DialContext(ctx, "tcp", raddr.String())
				if err != nil {
					log.Print(err)
					continue
				}
				defer rconn.Close()

				log.Print(fmt.Sprintf("connection from %s to %s (%s)", conn.RemoteAddr(), raddr.String(), proxy.String()))

				break
			}

			if err != nil {
				return
			}

			err = Bridge(conn, rconn)

			if err != nil {
				log.Print(err)
			}
		}()
	}
}

func Bridge(a, b io.ReadWriteCloser) error {
	done := make(chan error, 2)

	defer close(done)

	copy := func(a, b io.ReadWriteCloser, done chan error) {
		_, err := io.Copy(a, b)
		a.Close()
		b.Close()
		done <- err
	}

	go copy(a, b, done)
	go copy(b, a, done)

	err := <-done
	err2 := <-done
	if err2 == nil {
		err = nil
	}
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	return err
}

func (a *StringArray) String() string {
	return fmt.Sprintf("%v", *a)
}

func (a *StringArray) Set(value string) error {
	*a = append(*a, value)
	return nil
}
