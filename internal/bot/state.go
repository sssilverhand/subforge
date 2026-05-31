package bot

import (
	"sync"
	"time"
)

type stateKey int

const (
	stateIdle stateKey = iota
	stateAwaitingSubName
	stateAwaitingSubInbounds // future: inline multi-select
)

type userState struct {
	key       stateKey
	data      map[string]any
	updatedAt time.Time
}

// stateStore is an in-memory FSM store for multi-step bot flows.
// States expire after 10 minutes of inactivity.
type stateStore struct {
	mu     sync.Mutex
	states map[int64]*userState
}

func newStateStore() *stateStore {
	s := &stateStore{states: make(map[int64]*userState)}
	go s.gcLoop()
	return s
}

func (s *stateStore) set(chatID int64, key stateKey, data map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[chatID] = &userState{key: key, data: data, updatedAt: time.Now()}
}

func (s *stateStore) get(chatID int64) (stateKey, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.states[chatID]; ok {
		return st.key, st.data
	}
	return stateIdle, nil
}

func (s *stateStore) clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, chatID)
}

func (s *stateStore) gcLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, st := range s.states {
			if now.Sub(st.updatedAt) > 10*time.Minute {
				delete(s.states, id)
			}
		}
		s.mu.Unlock()
	}
}
