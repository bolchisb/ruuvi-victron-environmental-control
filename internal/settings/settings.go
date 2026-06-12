// Package settings stores the user-editable controller configuration: the two
// cooling stages (each with a custom name, an enable flag and a temperature
// setpoint), the thermostat deadband, the air-quality limits and the optional
// energy-aware gating. It is persisted as JSON under /data so it survives
// reboots and Venus OS firmware updates.
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

// DeratingThresholdC is the ambient temperature (°C) at or below which Victron
// inverters still deliver full output; above it they begin thermal derating.
// From Victron's technical note "Output rating, operating temperature and
// efficiency" (Rev 04): inverter continuous output is 100% up to 30 °C and
// drops from there (96% at 35 °C, 93% at 40 °C). The per-stage start
// temperatures default from this value.
const DeratingThresholdC = 30.0

// Stage is one cooling stage the controller can switch. Setpoint is the
// temperature (°C) at or above which the stage runs. Staging falls out of the
// setpoint ordering: stage 1 (cheap) has the lower setpoint and engages first;
// stage 2 (expensive) has a higher setpoint and only engages when the room
// climbs past it, i.e. when stage 1 could not hold the temperature. The setpoint
// defaults from the derating threshold and is editable in the UI.
type Stage struct {
	Name     string  `json:"name"`
	Enabled  bool    `json:"enabled"`
	Setpoint float64 `json:"setpoint"`
}

// AirQuality holds the optional CO2/NOX alarm limits. When Enabled and a Ruuvi
// Air sensor reports a value over a limit, the controller forces stage 1
// (ventilation) on to evacuate the gas — regardless of whether stage 1 cooling
// is enabled — and raises an alarm.
type AirQuality struct {
	Enabled  bool    `json:"enabled"`
	CO2Limit float64 `json:"co2Limit"`
	NOXLimit float64 `json:"noxLimit"`
}

// Energy holds the energy-aware gating, which always applies: a stage is only
// permitted to run while there is enough solar surplus (PV power minus loads) to
// cover it and the battery is above SocFloor: Stage1SurplusW permits the cheap
// stage, Stage2SurplusW the expensive one. Below that the controller does not
// cool from the grid until the room reaches GridCoolTemp while the inverter is
// also under sustained heavy load — its AC output at or above LoadTriggerW for at
// least LoadSustainMin minutes, the point at which the room will keep heating. A
// hot reading on its own (low load) is not worth grid energy, since the inverter
// only derates under load. When both hold, hardware protection overrides cost and
// every stage is permitted. These values are not exposed in the UI; they are
// fixed in code from defaults().
type Energy struct {
	SocFloor       float64 `json:"socFloor"`
	Stage1SurplusW float64 `json:"stage1SurplusW"`
	Stage2SurplusW float64 `json:"stage2SurplusW"`
	GridCoolTemp   float64 `json:"gridCoolTemp"`
	LoadTriggerW   float64 `json:"loadTriggerW"`
	LoadSustainMin float64 `json:"loadSustainMin"`
}

// Settings is the full persisted configuration.
type Settings struct {
	Stages   []Stage    `json:"stages"`
	Deadband float64    `json:"deadband"`
	Air      AirQuality `json:"air"`
	Energy   Energy     `json:"energy"`
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
			// Stage 1 (cheap exhaust) starts a couple of degrees before the
			// derating line to hold the room under it; stage 2 (AC) escalates at
			// the line itself.
			{Name: "Stage 1 cooling", Enabled: false, Setpoint: DeratingThresholdC - 2},
			{Name: "Stage 2 cooling", Enabled: false, Setpoint: DeratingThresholdC},
		},
		Deadband: 1.0,
		Air:      AirQuality{Enabled: false, CO2Limit: 1000, NOXLimit: 150},
		Energy: Energy{
			SocFloor:       50,
			Stage1SurplusW: 100,
			Stage2SurplusW: 1500,
			GridCoolTemp:   50,
			LoadTriggerW:   2000,
			LoadSustainMin: 10,
		},
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
	if in.Energy.SocFloor > 0 {
		out.Energy.SocFloor = in.Energy.SocFloor
	}
	if in.Energy.Stage1SurplusW > 0 {
		out.Energy.Stage1SurplusW = in.Energy.Stage1SurplusW
	}
	if in.Energy.Stage2SurplusW > 0 {
		out.Energy.Stage2SurplusW = in.Energy.Stage2SurplusW
	}
	if in.Energy.GridCoolTemp > 0 {
		out.Energy.GridCoolTemp = in.Energy.GridCoolTemp
	}
	if in.Energy.LoadTriggerW > 0 {
		out.Energy.LoadTriggerW = in.Energy.LoadTriggerW
	}
	if in.Energy.LoadSustainMin > 0 {
		out.Energy.LoadSustainMin = in.Energy.LoadSustainMin
	}
	return out
}

func clone(s Settings) Settings {
	c := s
	c.Stages = make([]Stage, len(s.Stages))
	copy(c.Stages, s.Stages)
	return c
}
