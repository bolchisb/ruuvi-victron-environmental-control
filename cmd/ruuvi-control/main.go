// Command ruuvi-control is the on-Cerbo controller: it reads Venus telemetry
// and sensor data over D-Bus, runs the staged cooling loop that drives the
// relays, and serves the embedded web UI.
package main

import (
	"log"
	"path/filepath"

	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/actuator"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/config"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/control"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/loadavg"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/peaks"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/settings"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/venus"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/web"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg := config.Load()

	bus, err := venus.Connect()
	if err != nil {
		log.Printf("d-bus unavailable, continuing without live telemetry: %v", err)
	}
	defer bus.Close()

	store, err := settings.Load(cfg.ConfigPath)
	if err != nil {
		log.Printf("settings: %v", err)
	}

	// The Cerbo GX has two on-board relays: stage 1 -> relay 1, stage 2 -> relay 2.
	relays := []actuator.Actuator{
		actuator.NewCerboRelay(bus, 0),
		actuator.NewCerboRelay(bus, 1),
	}

	// The rolling load average and the gauge peaks live next to the settings file
	// under /data so they survive a restart or a firmware update.
	dataDir := filepath.Dir(cfg.ConfigPath)
	loadAvg := loadavg.Load(filepath.Join(dataDir, "loadavg.json"))

	ctrl := control.New(bus, relays, store, loadAvg)
	go ctrl.Run()

	peakStore := peaks.Load(filepath.Join(dataDir, "peaks.json"))

	srv := web.NewServer(cfg, bus, relays, store, ctrl, peakStore, version)
	log.Printf("ruuvi-control %s started, UI on :%s", version, cfg.UIPort)
	log.Fatal(srv.Run())
}
