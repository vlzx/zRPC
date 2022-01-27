package xclient

import (
	"errors"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect = iota
	RoundRobinSelect
)

type Discovery interface {
	Refresh() error
	Update(servers []string) error
	Get(mode SelectMode) (string, error)
	GetAll() ([]string, error)
}

// MultiServerDiscovery manual discovery without a registry center
type MultiServerDiscovery struct {
	r       *rand.Rand
	mux     sync.RWMutex
	servers []string
	index   int
}

func NewMultiServerDiscovery(servers []string) *MultiServerDiscovery {
	msd := &MultiServerDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	msd.index = msd.r.Intn(math.MaxInt32 - 1)
	return msd
}

var _ Discovery = (*MultiServerDiscovery)(nil)

func (msd *MultiServerDiscovery) Refresh() error {
	return nil
}

func (msd *MultiServerDiscovery) Update(servers []string) error {
	msd.mux.Lock()
	defer msd.mux.Unlock()
	msd.servers = servers
	return nil
}

func (msd *MultiServerDiscovery) Get(mode SelectMode) (string, error) {
	msd.mux.Lock()
	defer msd.mux.Unlock()
	n := len(msd.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: no available servers")
	}
	switch mode {
	case RandomSelect:
		return msd.servers[msd.r.Intn(n)], nil
	case RoundRobinSelect:
		s := msd.servers[msd.index%n] // servers num could change, mod n to avoid out of range error
		msd.index = (msd.index + 1) % n
		return s, nil
	default:
		return "", errors.New("rpc discovery: unsupported select mode")
	}
}

func (msd *MultiServerDiscovery) GetAll() ([]string, error) {
	msd.mux.RLock()
	defer msd.mux.RUnlock()
	servers := make([]string, len(msd.servers), len(msd.servers))
	copy(servers, msd.servers)
	return servers, nil
}

type RegistryDiscovery struct {
	*MultiServerDiscovery
	registry   string
	timeout    time.Duration
	lastUpdate time.Time
}

const defaultUpdateTimeout = time.Second * 10

func NewRegistryDiscovery(registryAddr string, timeout time.Duration) *RegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	d := &RegistryDiscovery{
		MultiServerDiscovery: NewMultiServerDiscovery(make([]string, 0)),
		registry:             registryAddr,
		timeout:              timeout,
	}
	return d
}

func (d *RegistryDiscovery) Update(servers []string) error {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.servers = servers
	d.lastUpdate = time.Now()
	return nil
}

func (d *RegistryDiscovery) Refresh() error {
	d.mux.Lock()
	defer d.mux.Unlock()
	if d.lastUpdate.Add(d.timeout).After(time.Now()) {
		return nil
	}
	log.Println("rpc client: refresh servers from registry", d.registry)
	resp, err := http.Get(d.registry)
	if err != nil {
		log.Println("rpc registry refresh error", err)
		return err
	}
	servers := strings.Split(resp.Header.Get("X-Zrpc-Servers"), ",")
	d.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server != "" {
			d.servers = append(d.servers, server)
		}
	}
	d.lastUpdate = time.Now()
	return nil
}

func (d *RegistryDiscovery) Get(mode SelectMode) (string, error) {
	err := d.Refresh()
	if err != nil {
		return "", err
	}
	return d.MultiServerDiscovery.Get(mode)
}

func (d *RegistryDiscovery) GetAll() ([]string, error) {
	err := d.Refresh()
	if err != nil {
		return nil, err
	}
	return d.MultiServerDiscovery.GetAll()
}
