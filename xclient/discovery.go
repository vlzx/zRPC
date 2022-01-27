package xclient

import (
	"errors"
	"math"
	"math/rand"
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
