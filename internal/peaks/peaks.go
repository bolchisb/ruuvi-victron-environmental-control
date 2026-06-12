// Package peaks tracks a slowly-decaying per-metric maximum for the overview
// gauges, so each side arc fills against its own historic peak (the way the
// Victron GUI auto-scales each flow) instead of a shared instantaneous maximum.
//
// The peaks are persisted as JSON under /data so the gauge scale survives a
// service restart or a reboot and is shared across browser clients.
package peaks

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"sync"
	"time"
)

// tau is the decay time constant (seconds). Each tick a peak relaxes by
// e^(-dt/tau) toward the live value but never below it, so a transient spike
// fades over roughly ten minutes rather than flattening the gauge forever.
const tau = 900

// peakFloor keeps a peak just above zero so a metric that has been idle does not
// divide by zero on the front end.
const peakFloor = 1.0

// saveInterval throttles how often the peaks are written back to disk, to avoid
// hammering the GX flash on every status poll.
const saveInterval = 60 * time.Second

// Store holds the per-metric peaks. It is safe for concurrent use.
type Store struct {
	path        string
	mu          sync.Mutex
	data        map[string]float64
	lastObserve time.Time
	lastSave    time.Time
}

// Load reads the peaks file at path. A missing file is not an error: an empty
// set is returned and the peaks build up from the first observations.
func Load(path string) *Store {
	s := &Store{path: path, data: map[string]float64{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil {
		return s
	}
	var loaded map[string]float64
	if err := json.Unmarshal(b, &loaded); err == nil && loaded != nil {
		s.data = loaded
	}
	return s
}

// Observe decays the stored peaks toward the live magnitudes, raises any peak a
// magnitude now exceeds, and returns the current peaks. It persists at most once
// per saveInterval. mags is keyed by metric name with absolute (non-negative)
// values; a metric absent from mags still decays.
func (s *Store) Observe(now time.Time, mags map[string]float64) map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.lastObserve.IsZero() {
		if dt := now.Sub(s.lastObserve).Seconds(); dt > 0 {
			factor := math.Exp(-dt / tau)
			for k, v := range s.data {
				s.data[k] = v * factor
			}
		}
	}
	s.lastObserve = now

	for k, m := range mags {
		if m > s.data[k] {
			s.data[k] = m
		}
		if s.data[k] < peakFloor {
			s.data[k] = peakFloor
		}
	}

	if s.lastSave.IsZero() || now.Sub(s.lastSave) >= saveInterval {
		s.lastSave = now
		s.save()
	}
	return s.snapshot()
}

func (s *Store) snapshot() map[string]float64 {
	out := make(map[string]float64, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

func (s *Store) save() {
	b, err := json.Marshal(s.data)
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, b, 0o644)
}
