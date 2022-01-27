package main

import (
	"context"
	"github.com/vlzx/zrpc"
	"github.com/vlzx/zrpc/xclient"
	"log"
	"net"
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

func startServer(addr chan string) {
	var foo Foo
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	server := zrpc.NewServer()
	//server.HandleHTTP() // HTTP

	err = server.Register(&foo)
	if err != nil {
		log.Fatal("register error:", err)
	}

	log.Println("start rpc server on", listener.Addr())
	addr <- listener.Addr().String()
	server.Accept(listener) // TCP
	//_ = http.Serve(listener, nil) // HTTP
}

func call(addr chan string) {
	client, err := zrpc.XDial("tcp@" + <-addr) // TCP
	//client, err := zrpc.XDial("http@" + <-addr) // HTTP
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

func xCall(addr1 string, addr2 string) {
	msd := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(msd, xclient.RandomSelect, nil)
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

func xBroadcast(addr1 string, addr2 string) {
	msd := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(msd, xclient.RandomSelect, nil)
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
	//addr := make(chan string)
	//go startServer(addr)
	//call(addr)

	ch1, ch2 := make(chan string), make(chan string)
	go startServer(ch1)
	go startServer(ch2)
	addr1, addr2 := <-ch1, <-ch2
	time.Sleep(time.Second)
	xCall(addr1, addr2)
	xBroadcast(addr1, addr2)
}
