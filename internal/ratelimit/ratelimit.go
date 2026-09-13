package ratelimit

import (
	"sync"
	"time"
)

type Gate interface {
	Allow(key string, limit int, window time.Duration) (bool, time.Duration)
}

type entry struct {
	count int
	reset time.Time
}

type Limiter struct {
	mu      sync.Mutex
	entries map[string]entry
	now     func() time.Time
}

func New() *Limiter {
	return &Limiter{entries: make(map[string]entry), now: time.Now}
}

func (l *Limiter) Allow(key string, limit int, window time.Duration) (bool, time.Duration) {
	if limit <= 0 || window <= 0 {
		return false, window
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	item, ok := l.entries[key]
	if !ok || !now.Before(item.reset) {
		l.entries[key] = entry{count: 1, reset: now.Add(window)}
		l.prune(now)
		return true, window
	}
	if item.count >= limit {
		return false, item.reset.Sub(now)
	}
	item.count++
	l.entries[key] = item
	return true, item.reset.Sub(now)
}

func (l *Limiter) prune(now time.Time) {
	if len(l.entries) < 1024 {
		return
	}
	for key, item := range l.entries {
		if !now.Before(item.reset) {
			delete(l.entries, key)
		}
	}
}
