package maintenance

import "sync"

type keyedLock struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

type lockEntry struct {
	mu    sync.Mutex
	users int
}

func (locks *keyedLock) lock(key string) func() {
	locks.mu.Lock()
	if locks.entries == nil {
		locks.entries = make(map[string]*lockEntry)
	}
	entry := locks.entries[key]
	if entry == nil {
		entry = &lockEntry{}
		locks.entries[key] = entry
	}
	entry.users++
	locks.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		locks.mu.Lock()
		entry.users--
		if entry.users == 0 {
			delete(locks.entries, key)
		}
		locks.mu.Unlock()
	}
}
