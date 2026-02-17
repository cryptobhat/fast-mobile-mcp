package snapshot

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

type Bounds struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type Node struct {
	RefID       string
	ParentRefID string
	Index       int32
	Text        string
	ContentDesc string
	ResourceID  string
	ClassName   string
	PackageName string
	Bounds      Bounds
	Enabled     bool
	Clickable   bool
	Focusable   bool
	Visible     bool
	Selected    bool
	Checked     bool
}

type Snapshot struct {
	ID        string
	DeviceID  string
	CreatedAt time.Time
	ExpiresAt time.Time
	Nodes     []Node
}

type Store struct {
	mu                    sync.RWMutex
	items                 map[string]Snapshot
	latestByDevice        map[string]string
	byDevice              map[string][]string
	ttl                   time.Duration
	maxSnapshotsPerDevice int
	stop                  chan struct{}
}

func NewStore(ttl, cleanupInterval time.Duration, maxSnapshotsPerDevice int) *Store {
	s := &Store{
		items:                 make(map[string]Snapshot),
		latestByDevice:        make(map[string]string),
		byDevice:              make(map[string][]string),
		ttl:                   ttl,
		maxSnapshotsPerDevice: maxSnapshotsPerDevice,
		stop:                  make(chan struct{}),
	}

	go s.cleanupLoop(cleanupInterval)
	return s
}

func (s *Store) Close() {
	close(s.stop)
}

func (s *Store) Put(deviceID string, nodes []Node) Snapshot {
	now := time.Now().UTC()
	id := fmt.Sprintf("%s-%d", deviceID, now.UnixNano())
	snap := Snapshot{
		ID:        id,
		DeviceID:  deviceID,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
		Nodes:     append([]Node(nil), nodes...),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[id] = snap
	s.latestByDevice[deviceID] = id
	s.byDevice[deviceID] = append(s.byDevice[deviceID], id)

	if len(s.byDevice[deviceID]) > s.maxSnapshotsPerDevice {
		oldest := s.byDevice[deviceID][0]
		s.byDevice[deviceID] = s.byDevice[deviceID][1:]
		delete(s.items, oldest)
	}

	return snap
}

func (s *Store) Get(snapshotID string) (Snapshot, bool) {
	s.mu.RLock()
	snap, ok := s.items[snapshotID]
	s.mu.RUnlock()
	if !ok || time.Now().UTC().After(snap.ExpiresAt) {
		return Snapshot{}, false
	}
	return snap, true
}

func (s *Store) Latest(deviceID string) (Snapshot, bool) {
	s.mu.RLock()
	id, ok := s.latestByDevice[deviceID]
	s.mu.RUnlock()
	if !ok {
		return Snapshot{}, false
	}
	return s.Get(id)
}

func (s *Store) ResolveRef(snapshotID, refID string) (Node, bool) {
	snap, ok := s.Get(snapshotID)
	if !ok {
		return Node{}, false
	}
	for _, n := range snap.Nodes {
		if n.RefID == refID {
			return n, true
		}
	}
	return Node{}, false
}

func (s *Store) Page(snapshotID string, cursor string, limit int) (nodes []Node, nextCursor string, total int, ok bool) {
	snap, exists := s.Get(snapshotID)
	if !exists {
		return nil, "", 0, false
	}

	total = len(snap.Nodes)
	if total == 0 {
		return []Node{}, "", 0, true
	}

	start := 0
	if cursor != "" {
		v, err := strconv.Atoi(cursor)
		if err == nil && v >= 0 {
			start = v
		}
	}
	if start >= total {
		return []Node{}, "", total, true
	}

	if limit <= 0 {
		limit = 200
	}
	end := start + limit
	if end > total {
		end = total
	}

	paged := append([]Node(nil), snap.Nodes[start:end]...)
	if end < total {
		nextCursor = strconv.Itoa(end)
	}
	return paged, nextCursor, total, true
}

func (s *Store) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpired()
		case <-s.stop:
			return
		}
	}
}

func (s *Store) cleanupExpired() {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, snap := range s.items {
		if now.After(snap.ExpiresAt) {
			delete(s.items, id)
		}
	}

	for deviceID, ids := range s.byDevice {
		filtered := ids[:0]
		for _, id := range ids {
			if _, ok := s.items[id]; ok {
				filtered = append(filtered, id)
			}
		}
		s.byDevice[deviceID] = filtered
		if len(filtered) == 0 {
			delete(s.latestByDevice, deviceID)
			continue
		}
		s.latestByDevice[deviceID] = filtered[len(filtered)-1]
	}
}
