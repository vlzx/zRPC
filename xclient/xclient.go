package xclient

import (
	"context"
	"errors"
	. "github.com/vlzx/zrpc"
	"io"
	"reflect"
	"sync"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *Option
	mux     sync.Mutex // protect clients
	clients map[string]*Client
}

func (xc *XClient) Close() error {
	xc.mux.Lock()
	defer xc.mux.Unlock()
	for key, client := range xc.clients {
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, opt *Option) *XClient {
	return &XClient{
		d:       d,
		mode:    mode,
		opt:     opt,
		clients: make(map[string]*Client),
	}
}

func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	xc.mux.Lock()
	defer xc.mux.Unlock()
	client, ok := xc.clients[rpcAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(xc.clients, rpcAddr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = XDial(rpcAddr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, serviceMethod string, args interface{}, reply interface{}, ctx ...context.Context) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(serviceMethod, args, reply, ctx...)
}

func (xc *XClient) Call(serviceMethod string, args interface{}, reply interface{}, ctx ...context.Context) error {
	rpcAddr, err := xc.d.Get(xc.mode)
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, serviceMethod, args, reply, ctx...)
}

func (xc *XClient) Broadcast(serviceMethod string, args interface{}, reply interface{}, ctx ...context.Context) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mux sync.Mutex
	var e error
	replyDone := reply == nil
	if len(ctx) != 1 || ctx[0] == nil {
		return errors.New("rpc broadcast: invalid context")
	}
	cancelCtx, cancel := context.WithCancel(ctx[0])
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, serviceMethod, args, clonedReply, cancelCtx)
			mux.Lock()
			if err != nil && e == nil {
				e = err
				cancel() // if any call failed, cancel all unfinished calls
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mux.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	cancel()
	return e
}
