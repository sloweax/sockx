package proxy

import (
	"math/rand"
	"sync"
)

type Random struct {
	mutex  sync.RWMutex
	chains []Chain
}

func (r *Random) Add(c Chain) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.chains = append(r.chains, c)
}

func (r *Random) Next() Chain {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	chain := r.chains[rand.Int()%len(r.chains)]
	return chain
}

func (r *Random) Len() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.chains)
}

func (r *Random) All() []Chain {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	tmp := make([]Chain, 0, len(r.chains))
	tmp = append(tmp, r.chains...)
	return tmp
}
