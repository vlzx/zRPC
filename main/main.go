package main

import (
	"github.com/vlzx/zrpc"
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

func (f Foo) SumSlice(s []int, reply *int) error {
	for _, elem := range s {
		*reply += elem
	}
	return nil
}

func startServer(addr chan string) {
	var foo Foo
	err := zrpc.Register(&foo)
	if err != nil {
		log.Fatal("register error:", err)
	}
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", listener.Addr())
	addr <- listener.Addr().String()
	zrpc.Accept(listener)
}

func main() {
	addr := make(chan string)
	go startServer(addr)
	client, _ := zrpc.Dial("tcp", <-addr)
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
	go func() {
		var reply int
		err := client.Call("Foo.SumSlice", s, &reply)
		if err != nil {
			log.Fatal("call Foo.SumSlice error:", err)
		}
		log.Printf("sum %v = %d", s, reply)
	}()
	wg.Wait()
}
