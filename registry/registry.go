package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	timeout time.Duration
	mux     sync.Mutex
	servers map[string]*ServerNode
}

type ServerNode struct {
	Addr  string
	start time.Time
}

const (
	defaultPath    = "/_zrpc_/registry"
	defaultTimeout = time.Minute * 5
)

func NewRegistry(timeout time.Duration) *Registry {
	return &Registry{
		servers: make(map[string]*ServerNode),
		timeout: timeout,
	}
}

var DefaultRegistry = NewRegistry(defaultTimeout)

func (r *Registry) putServer(addr string) {
	r.mux.Lock()
	defer r.mux.Unlock()
	s := r.servers[addr]
	if s == nil {
		r.servers[addr] = &ServerNode{Addr: addr, start: time.Now()}
	} else {
		s.start = time.Now()
	}
}

func (r *Registry) aliveServers() []string {
	r.mux.Lock()
	defer r.mux.Unlock()
	var alive []string
	for addr, s := range r.servers {
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		w.Header().Set("X-Zrpc-Servers", strings.Join(r.aliveServers(), ","))
	case "POST":
		addr := req.Header.Get("X-Zrpc-Server")
		log.Println("receive heartbeat:", addr)
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *Registry) HandleHTTP() *http.ServeMux {
	handler := http.NewServeMux()
	handler.Handle(defaultPath, r)
	log.Println("rpc registry path:", defaultPath)
	return handler
}

func HandleHTTP() *http.ServeMux {
	return DefaultRegistry.HandleHTTP()
}

func Heartbeat(registry string, addr string, duration time.Duration) {
	if duration == 0 {
		duration = defaultTimeout - time.Minute
	}
	var err error
	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			err = sendHeartbeat(registry, addr)
			<-t.C
		}
	}()
}

func sendHeartbeat(registry string, addr string) error {
	log.Println(addr, "send heartbeat to registry", registry)
	client := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	log.Println("send heartbeat", addr)
	req.Header.Set("X-Zrpc-Server", addr)
	_, err := client.Do(req)
	if err != nil {
		log.Println("rpc server: heartbeat error:", err)
		return err
	}
	return nil
}
