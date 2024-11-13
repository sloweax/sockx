package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sloweax/argparse"
	"github.com/sloweax/sockx/proxy"
	"github.com/sloweax/sockx/proxy/socks5"
)

type Config struct {
	Verbose     bool
	Retry       uint     `name:"r" alias:"retry" metavar:"num" description:"if proxy connection fails, retry with another one up to num times"`
	Addr        string   `name:"a" alias:"addr" metavar:"addr[:port]" description:"listen on addr (default: 127.0.0.1:1080)"`
	Network     string   `name:"n" alias:"network" metavar:"name" description:"listen on network. available options: tcp, unix (default: tcp)"`
	Picker      string   `name:"p" alias:"picker" metavar:"name" description:"chain picker. available options: round-robin, random (default: round-robin)"`
	ConfigFiles []string `type:"positional" name:"file" metavar:"file..." description:"load config from file"`
}

func main() {

	rand.Seed(time.Now().Unix())

	config := Config{
		Addr:    "127.0.0.1:1080",
		Network: "tcp",
		Picker:  "round-robin",
	}

	parser := argparse.FromStruct(&config)

	if err := parser.ParseArgs(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var picker proxy.ChainPicker

	switch config.Picker {
	case "round-robin":
		picker = &proxy.RoundRobin{}
	case "random":
		picker = &proxy.Random{}
	default:
		log.Fatalf("unknown picker %q", config.Picker)
	}

	for _, file := range config.ConfigFiles {
		var f *os.File
		var err error

		if file == "-" {
			f = os.Stdin
		} else {
			f, err = os.Open(file)
			if err != nil {
				log.Fatal(err)
			}
		}

		if err := proxy.LoadPicker(picker, f); err != nil {
			log.Fatal(err)
		}

		f.Close()
	}

	if len(config.ConfigFiles) == 0 {
		log.Print("no specified config files, reading from stdin")
		if err := proxy.LoadPicker(picker, os.Stdin); err != nil {
			log.Fatal(err)
		}
	}

	if picker.Len() == 0 {
		log.Fatal("no loaded proxies")
	}

	if config.Verbose {
		for i, ps := range picker.All() {
			chain := make([]string, len(ps))
			for i, p := range ps {
				chain[i] = p.String()
			}
			log.Printf("chain %d: %s", i, strings.Join(chain, " | "))
		}
	}

	server := socks5.NewServer()
	err := server.Listen(config.Network, config.Addr)
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

			for i := uint(0); i <= config.Retry; i++ {
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
