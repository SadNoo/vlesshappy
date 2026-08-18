// vlesshappy-mysql-faultproxy is an acceptance-only TCP proxy that forwards a
// MySQL COMMIT and drops its response. It reproduces the ambiguous commit case
// that the traffic batch idempotency table must survive.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:13306", "local listen address")
	upstream := flag.String("upstream", "127.0.0.1:3306", "MySQL upstream address")
	flag.Parse()
	if flag.NArg() != 0 {
		fatal(errors.New("unexpected arguments"))
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		fatal(err)
	}
	defer listener.Close()
	for {
		client, err := listener.Accept()
		if err != nil {
			fatal(err)
		}
		go handle(client, *upstream)
	}
}

func handle(client net.Conn, upstreamAddress string) {
	defer client.Close()
	server, err := net.Dial("tcp", upstreamAddress)
	if err != nil {
		return
	}
	defer server.Close()
	var drop atomic.Bool
	var once sync.Once
	closeBoth := func() { once.Do(func() { client.Close(); server.Close() }) }
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			packet, payload, err := readPacket(client)
			if err != nil {
				return
			}
			if len(payload) > 1 && payload[0] == 0x03 && strings.EqualFold(strings.TrimSpace(string(payload[1:])), "COMMIT") {
				drop.Store(true)
			}
			if _, err := server.Write(packet); err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			packet, _, err := readPacket(server)
			if err != nil {
				return
			}
			if drop.Swap(false) {
				return
			}
			if _, err := client.Write(packet); err != nil {
				return
			}
		}
	}()
	<-done
	closeBoth()
	<-done
}

func readPacket(reader io.Reader) ([]byte, []byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, nil, err
	}
	length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if length < 0 || length > 16<<20 {
		return nil, nil, errors.New("invalid MySQL packet length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, nil, err
	}
	packet := make([]byte, 4+length)
	copy(packet, header)
	copy(packet[4:], payload)
	return packet, payload, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "vlesshappy-mysql-faultproxy:", err)
	os.Exit(1)
}
