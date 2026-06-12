// Package settings stores the user-editable controller configuration: the two
// cooling stages (each with a custom name, an enable flag and a temperature
// setpoint), the thermostat deadband and the air-quality limits. It is
// persisted as JSON under /data so it survives reboots and Venus OS firmware
// updates.
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

// Stage is one cooling stage the controller can switch. Setpoint is the
// temperature (°C) at or above which the stage runs. Staging falls out of the
// setpoint ordering: stage 1 (cheap) has the lower setpoint and engages first;
// stage 2 (expensive) has a higher setpoint and only engages when the room
// climbs past it, i.e. when stage 1 could not hold the temperature.
type Stage struct {
	Name     string  `json:"name"`
	Enabled  bool    `json:"enabled"`
	Setpoint float64 `json:"setpoint"`
}

// AirQuality holds the optional CO2/NOX alarm limits. When Enabled and a Ruuvi
// Air sensor reports a value over a limit, the controller forces stage 1
// (ventilation) on and raises an alarm.
type AirQuality struct {
	Enabled  bool    `json:"enabled"`
	CO2Limit float64 `json:"co2Limit"`
	NOXLimit float64 `json:"noxLimit"`
}

// Settings is the full persisted configuration.
type Settings struct {
	Stages   []Stage    `json:"stages"`
	Deadband float64    `json:"deadband"`
	Air      AirQuality `json:"air"`
}

// Store loads, holds and persists Settings. It is safe for concurrent use.
type Store struct {
	path string
	mu   sync.RWMutex
	data Settings
}

func defaults() Settings {
	return Settings{
		Stages: []Stage{
			{Name: "Stage 1 cooling", Enabled: false, Setpoint: 28},
			{Name: "Stage 2 cooling", Enabled: false, Setpoint: 31},
		},
		Deadband: 1.0,
		Air:      AirQuality{Enabled: false, CO2Limit: 1000, NOXLimit: 150},
	}
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

// normalize forces exactly stageCount stages, trims names, and falls back to
// the defaults for any blank name or non-positive numeric field, so a partial
// or malformed payload can never disable cooling silently.
func normalize(in Settings) Settings {
	def := defaults()
	out := def
	out.Stages = make([]Stage, stageCount)
	for i := 0; i < stageCount; i++ {
		out.Stages[i] = def.Stages[i]
		if i < len(in.Stages) {
			if name := strings.TrimSpace(in.Stages[i].Name); name != "" {
				out.Stages[i].Name = name
			}
			out.Stages[i].Enabled = in.Stages[i].Enabled
			if in.Stages[i].Setpoint > 0 {
				out.Stages[i].Setpoint = in.Stages[i].Setpoint
			}
		}
	}
	if in.Deadband > 0 {
		out.Deadband = in.Deadband
	}
	out.Air.Enabled = in.Air.Enabled
	if in.Air.CO2Limit > 0 {
		out.Air.CO2Limit = in.Air.CO2Limit
	}
	if in.Air.NOXLimit > 0 {
		out.Air.NOXLimit = in.Air.NOXLimit
	}
	return out
}

func clone(s Settings) Settings {
	c := s
	c.Stages = make([]Stage, len(s.Stages))
	copy(c.Stages, s.Stages)
	return c
}
