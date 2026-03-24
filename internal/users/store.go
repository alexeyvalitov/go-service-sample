package users

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"time"
)

var ErrNotFound = errors.New("user not found")

type Store struct {
	mu    sync.RWMutex
	users map[string]User
}

func NewStore() *Store {
	return &Store{users: make(map[string]User)}
}

func (s *Store) List() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out
}

func (s *Store) Get(id string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (s *Store) Create(name string) User {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := newID()
	u := User{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	s.users[id] = u
	return u
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	delete(s.users, id)
	return nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "t" + strconv.FormatInt(time.Now().UTC().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
