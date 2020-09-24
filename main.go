package main

import (
	"context"
	"flag"
	"net"

	"github.com/armon/go-socks5"
	"github.com/jursonmo/tcpbinddev"
)

var device string

func init() {
	flag.StringVar(&device, "device", "mangler", "network interface to use")
	flag.Parse()
}

func main() {
	conf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			h, _, err := net.SplitHostPort(addr)
			if nil == err {
				ip := net.ParseIP(h)
				if ip != nil {
					if ip.To4() != nil {
						network = network + "4"
					} else if ip.To16() != nil {
						network = network + "6"
					}
				}
			}
			return tcpbinddev.TcpBindToDev(network, addr, "", device, 10)
		},
	}
	server, err := socks5.New(conf)
	if err != nil {
		panic(err)
	}

	if err := server.ListenAndServe("tcp", "127.0.0.1:8888"); err != nil {
		panic(err)
	}
}
