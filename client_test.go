package zrpc

import (
	"context"
	"log"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClient_Connect(t *testing.T) {
	t.Parallel()
	l, _ := net.Listen("tcp", ":0")
	f := func(conn net.Conn, opt *Option) (client *Client, err error) {
		_ = conn.Close()
		time.Sleep(time.Second * 2)
		return nil, nil
	}
	t.Run("timeout", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &Option{ConnectTimeout: time.Second})
		log.Println(err)
		_assert(err != nil && strings.Contains(err.Error(), "connect timeout"), "expect a timeout error")
	})
	t.Run("timeout", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &Option{ConnectTimeout: 0})
		_assert(err == nil, "no timeout limit")
	})
}

type Bar struct{}

func (b Bar) Timeout(argv int, reply *int) error {
	time.Sleep(time.Second * 2)
	return nil
}

func startServer(addr chan string) {
	var b Bar
	_ = Register(&b)
	l, _ := net.Listen("tcp", ":0")
	addr <- l.Addr().String()
	Accept(l)
}

func TestClient_Call(t *testing.T) {
	t.Parallel()
	addrCh := make(chan string)
	go startServer(addrCh)
	addr := <-addrCh

	time.Sleep(time.Second)

	t.Run("client timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr)
		ctx, _ := context.WithTimeout(context.Background(), time.Second)
		var reply int
		err := client.Call("Bar.Timeout", 1, &reply, ctx)
		_assert(err != nil && strings.Contains(err.Error(), ctx.Err().Error()), "expect a timeout error")
	})
	t.Run("client timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr)
		var reply int
		err := client.Call("Bar.Timeout", 1, &reply)
		_assert(err == nil, "no timeout limit")
	})
	t.Run("server timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr)
		client.header.Timeout = time.Second
		var reply int
		err := client.Call("Bar.Timeout", 1, &reply)
		_assert(err != nil && strings.Contains(err.Error(), "handle request timeout"), "expect a timeout error")
	})
	t.Run("server timeout", func(t *testing.T) {
		client, _ := Dial("tcp", addr)
		var reply int
		err := client.Call("Bar.Timeout", 1, &reply)
		_assert(err == nil, "no timeout limit")
	})
}
