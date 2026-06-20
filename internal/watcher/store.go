// Package watcher simulates the Kubernetes API machinery that operators depend on.
package watcher

import (
	"fmt"
	"sync"
	"time"
)

// EventType mirrors watch.EventType in k8s.io/apimachinery.
type EventType string

const (
	EventAdded    EventType = "ADDED"
	EventModified EventType = "MODIFIED"
	EventDeleted  EventType = "DELETED"
)

// Event is what the watch stream delivers to controllers.
type Event struct {
	Type      EventType
	Key       string
	OldValue  interface{}
	NewValue  interface{}
	Timestamp time.Time
}

// Store is our etcd analogue — the source of truth for desired state.
type Store struct {
	mu          sync.RWMutex
	data        map[string]interface{}
	revision    int64
	subscribers []chan Event
}

func NewStore() *Store {
	return &Store{
		data:        make(map[string]interface{}),
		subscribers: make([]chan Event, 0),
	}
}

// Watch returns a channel that receives every state change in the store.
func (s *Store) Watch() <-chan Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan Event, 32)
	s.subscribers = append(s.subscribers, ch)
	return ch
}

// Set stores a value and notifies all watchers.
func (s *Store) Set(key string, value interface{}) {
	s.mu.Lock()
	old, exists := s.data[key]
	s.data[key] = value
	s.revision++
	rev := s.revision
	subs := make([]chan Event, len(s.subscribers))
	copy(subs, s.subscribers)
	s.mu.Unlock()

	eventType := EventAdded
	if exists {
		eventType = EventModified
	}

	evt := Event{Type: eventType, Key: key, OldValue: old, NewValue: value, Timestamp: time.Now()}
	fmt.Printf("[etcd] revision=%d  %s  key=%q\n", rev, eventType, key)

	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
			fmt.Printf("[etcd] WARNING: watcher too slow, event dropped for key=%q\n", key)
		}
	}
}

// Get retrieves current state.
func (s *Store) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// Delete removes a key and notifies watchers.
func (s *Store) Delete(key string) {
	s.mu.Lock()
	old, exists := s.data[key]
	if !exists {
		s.mu.Unlock()
		return
	}
	delete(s.data, key)
	s.revision++
	rev := s.revision
	subs := make([]chan Event, len(s.subscribers))
	copy(subs, s.subscribers)
	s.mu.Unlock()

	evt := Event{Type: EventDeleted, Key: key, OldValue: old, NewValue: nil, Timestamp: time.Now()}
	fmt.Printf("[etcd] revision=%d  DELETED  key=%q\n", rev, key)

	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

// Revision returns the current global revision counter.
func (s *Store) Revision() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision
}
