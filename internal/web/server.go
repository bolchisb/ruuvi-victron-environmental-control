// Package web serves the embedded gui-v2-themed UI and a small JSON API.
// All static assets are compiled into the binary via go:embed, so deployment
// is a single self-contained file.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/actuator"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/config"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/control"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/peaks"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/settings"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/venus"
)

//go:embed static
var staticFS embed.FS

// Server wires the UI and API to the bus and actuators.
type Server struct {
	cfg      config.Config
	bus      *venus.Bus
	relays   []actuator.Actuator
	settings *settings.Store
	ctrl     *control.Controller
	peaks    *peaks.Store
	version  string
}

// NewServer constructs the HTTP server.
func NewServer(cfg config.Config, bus *venus.Bus, relays []actuator.Actuator, store *settings.Store, ctrl *control.Controller, peakStore *peaks.Store, version string) *Server {
	return &Server{cfg: cfg, bus: bus, relays: relays, settings: store, ctrl: ctrl, peaks: peakStore, version: version}
}

// Run starts the HTTP listener (blocking).
func (s *Server) Run() error {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/relay", s.handleRelay)
	return http.ListenAndServe(":"+s.cfg.UIPort, mux)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	type output struct {
		Name string `json:"name"`
		On   *bool  `json:"on"`
	}
	type status struct {
		Version      string                   `json:"version"`
		BusConnected bool                     `json:"busConnected"`
		AirAlarm     bool                     `json:"airAlarm"`
		System       map[string]venus.Reading `json:"system"`
		Peaks        map[string]float64       `json:"peaks"`
		Sensors      []venus.Sensor           `json:"sensors"`
		Outputs      []output                 `json:"outputs"`
	}
	sensors, _ := s.bus.ReadSensors()
	system := s.bus.ReadSystem()
	out := status{
		Version:      s.version,
		BusConnected: s.bus.Connected(),
		AirAlarm:     s.ctrl.AirAlarm(),
		System:       system,
		Peaks:        s.peaks.Observe(time.Now(), flowMagnitudes(system)),
		Sensors:      sensors,
	}
	for _, r := range s.relays {
		o := output{Name: r.Name()}
		if on, err := r.State(); err == nil {
			o.On = &on
		}
		out.Outputs = append(out.Outputs, o)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleConfig serves the user's stage settings:
// GET returns the current settings; POST replaces and persists them.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Embed the researched derating threshold alongside the saved settings
		// so the UI can show it as the default the setpoints start from. POST
		// ignores it (it decodes into settings.Settings).
		type configResponse struct {
			settings.Settings
			DeratingThresholdC float64 `json:"deratingThresholdC"`
		}
		writeJSON(w, http.StatusOK, configResponse{
			Settings:           s.settings.Get(),
			DeratingThresholdC: settings.DeratingThresholdC,
		})
	case http.MethodPost:
		var in settings.Settings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		saved, err := s.settings.Update(in)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRelay is a v0 smoke-test endpoint to validate actuation:
// POST /api/relay?index=0&state=1
func (s *Server) handleRelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	index, err := strconv.Atoi(r.URL.Query().Get("index"))
	if err != nil || index < 0 || index >= len(s.relays) {
		http.Error(w, "invalid index", http.StatusBadRequest)
		return
	}
	on := r.URL.Query().Get("state") == "1"
	if err := s.relays[index].Set(on); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"output": s.relays[index].Name(), "on": on})
}

// flowMagnitudes pulls the four overview flows out of the system reading as
// absolute values, keyed for the peak tracker. A missing reading contributes 0,
// so its peak simply decays until the metric returns.
func flowMagnitudes(system map[string]venus.Reading) map[string]float64 {
	mags := make(map[string]float64, 4)
	for _, key := range []string{"pv_power", "grid", "ac_loads", "dc_loads"} {
		v := 0.0
		if r, ok := system[key]; ok && r.Value != nil {
			v = math.Abs(*r.Value)
		}
		mags[key] = v
	}
	return mags
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
