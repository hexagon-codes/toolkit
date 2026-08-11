package multi

import (
	"context"
	"sync"
)

type keyGate struct {
	token chan struct{}
	refs  int
}

type keyGateSet struct {
	mu    sync.Mutex
	gates map[string]*keyGate
}

func (s *keyGateSet) acquire(ctx context.Context, key string) (func(), error) {
	s.mu.Lock()
	if s.gates == nil {
		s.gates = make(map[string]*keyGate)
	}
	gate := s.gates[key]
	if gate == nil {
		gate = &keyGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		s.gates[key] = gate
	}
	gate.refs++
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.releaseReference(key, gate)
		return nil, ctx.Err()
	case <-gate.token:
		var once sync.Once
		return func() {
			once.Do(func() {
				gate.token <- struct{}{}
				s.releaseReference(key, gate)
			})
		}, nil
	}
}

func (s *keyGateSet) releaseReference(key string, gate *keyGate) {
	s.mu.Lock()
	gate.refs--
	if gate.refs == 0 && s.gates[key] == gate {
		delete(s.gates, key)
	}
	s.mu.Unlock()
}
