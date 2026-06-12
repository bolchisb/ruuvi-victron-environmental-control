// Package settings stores the user-editable controller configuration: the two
// cooling stages, each with a custom name and an enable flag. It is persisted
// as JSON under /data so it survives reboots and Venus OS firmware updates.
//
// The stage-to-relay mapping is fixed: stage 1 switches Cerbo relay 1 and
// stage 2 switches relay 2.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
)

// stageCount is fixed by the Cerbo's two on-board relays.
const stageCount = 2

// Stage is one cooling stage the controller can switch.
type Stage struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Settings is the full persisted configuration.
type Settings struct {
	Stages []Stage `json:"stages"`
}

// Store loads, holds and persists Settings. It is safe for concurrent use.
type Store struct {
	path string
	mu   sync.RWMutex
	data Settings
}

func defaults() Settings {
	return Settings{Stages: []Stage{
		{Name: "Stage 1 cooling", Enabled: false},
		{Name: "Stage 2 cooling", Enabled: false},
	}}
}

// Load reads the settings file at path. A missing file is not an error: the
// defaults are returned and written, so the file exists for later edits.
func Load(path string) (*Store, error) {
	s := &Store{path: path, data: defaults()}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, s.save()
	}
	if err != nil {
		return s, err
	}
	var loaded Settings
	if err := json.Unmarshal(b, &loaded); err != nil {
		return s, err
	}
	s.data = normalize(loaded)
	return s, nil
}

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.data)
}

// Update validates, stores and persists new settings, returning what was saved.
func (s *Store) Update(in Settings) (Settings, error) {
	n := normalize(in)
	s.mu.Lock()
	s.data = n
	err := s.save()
	out := clone(s.data)
	s.mu.Unlock()
	return out, err
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

// normalize forces exactly stageCount stages, trims names and falls back to the
// default name when a stage is left blank.
func normalize(in Settings) Settings {
	def := defaults()
	out := Settings{Stages: make([]Stage, stageCount)}
	for i := 0; i < stageCount; i++ {
		out.Stages[i] = def.Stages[i]
		if i < len(in.Stages) {
			if name := strings.TrimSpace(in.Stages[i].Name); name != "" {
				out.Stages[i].Name = name
			}
			out.Stages[i].Enabled = in.Stages[i].Enabled
		}
	}
	return out
}

func clone(s Settings) Settings {
	c := Settings{Stages: make([]Stage, len(s.Stages))}
	copy(c.Stages, s.Stages)
	return c
}
