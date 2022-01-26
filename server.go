package zrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vlzx/zrpc/codec"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MagicNumber = 0x2a

type Option struct {
	MagicNumber    int
	CodecType      string
	ConnectTimeout time.Duration
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: time.Second * 10,
}

type Server struct {
	serviceMap sync.Map
}

func (server *Server) Register(receiver interface{}) error {
	s := newService(receiver)
	_, dup := server.serviceMap.LoadOrStore(s.name, s)
	if dup {
		return errors.New("rpc server: service already exists: " + s.name)
	}
	return nil
}

func Register(receiver interface{}) error {
	return DefaultServer.Register(receiver)
}

func (server *Server) findService(serviceMethod string) (svc *service, mType *methodType, err error) {
	tokens := strings.Split(serviceMethod, ".")
	if len(tokens) != 2 {
		err = errors.New("rpc server: invalid name, should be service.method")
		return
	}
	serviceName, methodName := tokens[0], tokens[1]
	s, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can not find service " + serviceName)
		return
	}
	svc = s.(*service)
	mType = svc.method[methodName]
	if mType == nil {
		err = errors.New("rpc server: can not find method " + methodName)
	}
	return
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

func (server *Server) Accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go server.ServeConn(conn)
	}
}

func Accept(listener net.Listener) {
	DefaultServer.Accept(listener)
}

func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: option error:", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("zrpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("zrpc server: invalid codec type %s", opt.CodecType)
		return
	}
	server.ServeCodec(f(conn))
}

var invalidRequest = struct{}{}

func (server *Server) ServeCodec(c codec.Codec) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		req, err := server.readRequest(c)
		if err != nil {
			if req == nil {
				break
			}
			req.header.Error = err.Error()
			server.sendResponse(c, req.header, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go server.handleRequest(c, req, sending, wg)
	}
	wg.Wait()
	_ = c.Close()
}

type request struct {
	header *codec.Header
	argv   reflect.Value
	replyv reflect.Value
	mType  *methodType
	svc    *service
}

func (server *Server) readRequestHeader(c codec.Codec) (*codec.Header, error) {
	var header codec.Header
	if err := c.ReadHeader(&header); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &header, nil
}

func (server *Server) readRequest(c codec.Codec) (*request, error) {
	header, err := server.readRequestHeader(c)
	if err != nil {
		return nil, err
	}
	req := &request{header: header}
	req.svc, req.mType, err = server.findService(header.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mType.newArgv()
	req.replyv = req.mType.newReplyv()

	// make sure argvi (interface of argv) is a pointer
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	err = c.ReadBody(argvi)
	if err != nil {
		log.Println("rpc server: read argv error:", err)
		return req, err
	}
	return req, nil
}

func (server *Server) sendResponse(c codec.Codec, header *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := c.Write(header, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(c codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println(req.header, req.argv)
	called := make(chan struct{}, 1)
	sent := make(chan struct{}, 1)
	timeout := req.header.Timeout
	go func() {
		err := req.svc.call(req.mType, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.header.Error = err.Error()
			server.sendResponse(c, req.header, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		server.sendResponse(c, req.header, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()
	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		req.header.Error = fmt.Sprintf("rpc server: handle request timeout: expect within %s", timeout)
		server.sendResponse(c, req.header, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

const (
	connected        = "200 Connection to zRPC Established"
	defaultRPCPath   = "/_zrpc_"
	defaultDebugPath = "/debug/zrpc"
)

func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "Use CONNECT method\n")
		return
	}
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
	server.ServeConn(conn)
}

func (server *Server) HandleHTTP() {
	http.Handle(defaultRPCPath, server)
	http.Handle(defaultDebugPath, debugHTTP{server})
	log.Println("rpc server debug path:", defaultDebugPath)
}

func HandleHTTP() {
	DefaultServer.HandleHTTP()
}
