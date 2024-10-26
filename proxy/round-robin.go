package proxy

import (
	"bufio"
	"io"
	"strings"
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

func (r *RoundRobin) Load(f io.Reader) error {
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		fields, err := parseFields(line)
		if err != nil {
			return err
		}

		if len(fields) == 0 {
			continue
		}

		chain, err := parseChain(fields)
		if err != nil {
			return err
		}

		if len(chain) == 0 {
			continue
		}

		r.Add(chain)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func (r *RoundRobin) All() []Chain {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.chains
}
