// Package web serves the embedded gui-v2-themed UI and a small JSON API.
// All static assets are compiled into the binary via go:embed, so deployment
// is a single self-contained file.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/actuator"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/config"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/venus"
)

//go:embed static
var staticFS embed.FS

// Server wires the UI and API to the bus and actuators.
type Server struct {
	cfg     config.Config
	bus     *venus.Bus
	relays  []actuator.Actuator
	version string
}

// NewServer constructs the HTTP server.
func NewServer(cfg config.Config, bus *venus.Bus, relays []actuator.Actuator, version string) *Server {
	return &Server{cfg: cfg, bus: bus, relays: relays, version: version}
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
	mux.HandleFunc("/api/relay", s.handleRelay)
	return http.ListenAndServe(":"+s.cfg.UIPort, mux)
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	type status struct {
		Version      string                   `json:"version"`
		BusConnected bool                     `json:"busConnected"`
		System       map[string]venus.Reading `json:"system"`
		Outputs      []string                 `json:"outputs"`
	}
	out := status{
		Version:      s.version,
		BusConnected: s.bus.Connected(),
		System:       s.bus.ReadSystem(),
	}
	for _, r := range s.relays {
		out.Outputs = append(out.Outputs, r.Name())
	}
	writeJSON(w, http.StatusOK, out)
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
