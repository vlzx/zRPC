package main

import (
	"context"
	"github.com/vlzx/zrpc"
	"github.com/vlzx/zrpc/registry"
	"github.com/vlzx/zrpc/xclient"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Foo struct{}

type Args struct {
	Num1 int
	Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) SumSleep(args Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) SumSlice(s []int, reply *int) error {
	for _, elem := range s {
		*reply += elem
	}
	return nil
}

func startRegistry(wg *sync.WaitGroup) {
	listener, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatal("network error:", err)
	}
	handler := registry.HandleHTTP()
	wg.Done()
	_ = http.Serve(listener, handler)
}

func startServer(registryAddr string, wg *sync.WaitGroup) {
	var foo Foo
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	server := zrpc.NewServer()
	//handler := server.HandleHTTP() // HTTP

	err = server.Register(&foo)
	if err != nil {
		log.Fatal("register error:", err)
	}

	registry.Heartbeat(registryAddr, "tcp@"+listener.Addr().String(), 0)

	log.Println("start rpc server on", listener.Addr())
	wg.Done()
	server.Accept(listener) // TCP
	//_ = http.Serve(listener, handler) // HTTP
}

func call(addr chan string) {
	client, err := zrpc.XDial("tcp@" + <-addr) // TCP
	//client, err := zrpc.XDial("tcp@" + <-addr) // HTTP
	if err != nil {
		log.Fatal("dial error:", err)
	}
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	s := make([]int, 0)
	for i := 0; i < 5; i++ {
		s = append(s, i)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			var reply int
			err := client.Call("Foo.Sum", args, &reply)
			if err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		var reply int
		err := client.Call("Foo.SumSlice", s, &reply)
		if err != nil {
			log.Fatal("call Foo.SumSlice error:", err)
		}
		log.Printf("sum %v = %d", s, reply)
	}()
	wg.Wait()
}

func afterLog(xc *xclient.XClient, xcMethod string, serviceMethod string, args *Args, ctx ...context.Context) {
	var reply int
	var err error
	switch xcMethod {
	case "call":
		err = xc.Call(serviceMethod, args, &reply, ctx...)
	case "broadcast":
		err = xc.Broadcast(serviceMethod, args, &reply, ctx...)
	}
	if err != nil {
		log.Printf("%s %s error: %s", xcMethod, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", xcMethod, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func xCall(registryAddr string) {
	d := xclient.NewRegistryDiscovery(registryAddr, 0)
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() { _ = xc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			afterLog(xc, "call", "Foo.Sum", &Args{Num1: i, Num2: i * i}, context.Background())
		}(i)
	}
	wg.Wait()
}

func xBroadcast(registryAddr string) {
	d := xclient.NewRegistryDiscovery(registryAddr, 0)
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() { _ = xc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			afterLog(xc, "broadcast", "Foo.Sum", &Args{Num1: i, Num2: i * i}, context.Background())
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			afterLog(xc, "broadcast", "Foo.SumSleep", &Args{Num1: i, Num2: i * i}, ctx)
		}(i)
	}
	wg.Wait()
}

func main() {
	registryAddr := "http://localhost:9999/_zrpc_/registry"
	var wg sync.WaitGroup
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()

	time.Sleep(time.Second)

	wg.Add(2)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	wg.Wait()

	time.Sleep(time.Second)

	xCall(registryAddr)
	xBroadcast(registryAddr)
}
