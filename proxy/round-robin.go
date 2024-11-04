package proxy

import (
	"sync"
)

type RoundRobin struct {
	mutex  sync.RWMutex
	index  int
	chains []Chain
}

func (r *RoundRobin) Add(c Chain) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.chains = append(r.chains, c)
}

func (r *RoundRobin) Next() Chain {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	chain := r.chains[r.index%len(r.chains)]
	r.index += 1
	return chain
}

func (r *RoundRobin) Len() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.chains)
}

func (r *RoundRobin) All() []Chain {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.chains
}
