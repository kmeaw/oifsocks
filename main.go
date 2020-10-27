package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"

	"github.com/armon/go-socks5"
	"github.com/go-httpproxy/httpproxy"
	"github.com/jursonmo/tcpbinddev"
	"golang.org/x/sync/errgroup"
)

var device string
var http_port, socks_port int

func init() {
	flag.StringVar(&device, "device", "mangler", "network interface to use")
	flag.IntVar(&http_port, "http", 3128, "HTTP-proxy port")
	flag.IntVar(&socks_port, "socks", 8888, "SOCKS5 port")
	flag.Parse()
}

func DialFunc(ctx context.Context, network, addr string) (net.Conn, error) {
	h, _, err := net.SplitHostPort(addr)
	if nil == err {
		ip := net.ParseIP(h)
		if nil == ip {
			ips, err := net.LookupIP(h)
			if err != nil {
				return nil, fmt.Errorf("cannot resolve %s: %w", h, err)
			} else if len(ips) == 0 {
				return nil, fmt.Errorf("%s does not resolve to an IP address", h)
			} else {
				ip = ips[rand.Intn(len(ips))]
			}
		}
		if ip != nil {
			if ip.To4() != nil {
				network = network + "4"
			} else if ip.To16() != nil {
				network = network + "6"
			}
		}
	}
	return tcpbinddev.TcpBindToDev(network, addr, "", device, 10)
}

func OnError(ctx *httpproxy.Context, where string,
	err *httpproxy.Error, opErr error) {
	log.Printf("ERROR: %s: %s [%s]", where, err, opErr)
}

func OnAccept(ctx *httpproxy.Context, w http.ResponseWriter,
	r *http.Request) bool {
	log.Printf("HTTP %s %s", r.Method, r.URL)
	return false
}

func OnConnect(ctx *httpproxy.Context, host string) (httpproxy.ConnectAction, string) {
	log.Printf("HTTP CONNECT %s", host)
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Printf("cannot listen localhost:0: %s", err)
		return httpproxy.ConnectProxy, ""
	}
	go func() {
		defer l.Close()
		conn2, err := DialFunc(context.Background(), "tcp", host)
		if err != nil {
			log.Printf("cannot dial %q: %s", host, err)
			return
		}
		conn, err := l.Accept()
		if err != nil {
			log.Printf("cannot accept loopback connection: %s", err)
			return
		}
		if _, _, err := net.SplitHostPort(host); nil == err {
			host = host + ":80"
		}
		g := &errgroup.Group{}
		g.Go(func() error {
			_, err := io.Copy(conn, conn2)
			return err
		})
		g.Go(func() error {
			_, err := io.Copy(conn2, conn)
			return err
		})
		g.Wait()
		conn.Close()
		conn2.Close()
	}()
	return httpproxy.ConnectProxy, l.Addr().String()
}

func main() {
	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			log.Printf("SOCKS %s", addr)
			return DialFunc(ctx, network, addr)
		},
	}
	socks_server, err := socks5.New(conf)
	if err != nil {
		panic(err)
	}

	http_proxy, err := httpproxy.NewProxy()
	if err != nil {
		panic(err)
	}

	http_proxy.Rt = &http.Transport{
		DialContext: DialFunc,
	}
	http_proxy.OnError = OnError
	http_proxy.OnAccept = OnAccept
	http_proxy.OnConnect = OnConnect

	g := &errgroup.Group{}
	g.Go(func() error { return socks_server.ListenAndServe("tcp", fmt.Sprintf("127.0.0.1:%d", socks_port)) })
	g.Go(func() error { return http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", http_port), http_proxy) })

	panic(g.Wait())
}
