// Package loadavg keeps a rolling 24-hour mean of the inverter AC output, used
// as the self-calibrating "heavy load" threshold for the grid-cooling override
// instead of a fixed wattage. Samples are kept in 24 hourly buckets in a ring,
// so each new hour forgets the hour that is now 24 hours old.
//
// The window is persisted as JSON under /data so the average survives a service
// restart or a reboot.
package loadavg

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

// buckets is the window length in hourly slots.
const buckets = 24

// saveInterval throttles how often the window is written back to disk, to avoid
// hammering the GX flash on every sample.
const saveInterval = 60 * time.Second

type bucket struct {
	Sum   float64 `json:"sum"`
	Count int     `json:"count"`
}

type state struct {
	Buckets [buckets]bucket `json:"buckets"`
	Idx     int             `json:"idx"`
	Cur     time.Time       `json:"cur"` // start of the hour the current bucket holds
}

// Store holds the rolling window. It is safe for concurrent use.
type Store struct {
	path     string
	mu       sync.Mutex
	st       state
	lastSave time.Time
}

// Load reads the window file at path. A missing file is not an error: an empty
// window is returned and it fills from the first samples.
func Load(path string) *Store {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil {
		return s
	}
	var loaded state
	if err := json.Unmarshal(b, &loaded); err == nil {
		s.st = loaded
	}
	return s
}

// Add records one AC-output sample at time now, rotating hourly buckets so
// samples older than 24 hours fall out of the window. It persists at most once
// per saveInterval.
func (s *Store) Add(now time.Time, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.st.Cur.IsZero() {
		s.st.Cur = now.Truncate(time.Hour)
	}
	// Advance whole elapsed hours, clearing each bucket we land on so it forgets
	// the hour now 24 hours old. After long downtime, capping at the ring size
	// clears the whole window.
	adv := int(now.Sub(s.st.Cur) / time.Hour)
	if adv > buckets {
		adv = buckets
	}
	for i := 0; i < adv; i++ {
		s.st.Idx = (s.st.Idx + 1) % buckets
		s.st.Buckets[s.st.Idx] = bucket{}
	}
	if adv > 0 {
		s.st.Cur = s.st.Cur.Add(time.Duration(adv) * time.Hour)
	}

	s.st.Buckets[s.st.Idx].Sum += value
	s.st.Buckets[s.st.Idx].Count++

	if s.lastSave.IsZero() || now.Sub(s.lastSave) >= saveInterval {
		s.lastSave = now
		s.save()
	}
}

// Mean returns the mean AC output across the retained window and whether any
// sample is present.
func (s *Store) Mean() (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sum float64
	var count int
	for _, b := range s.st.Buckets {
		sum += b.Sum
		count += b.Count
	}
	if count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

func (s *Store) save() {
	b, err := json.Marshal(s.st)
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, b, 0o644)
}
