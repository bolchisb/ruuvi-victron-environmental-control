// Command ruuvi-control is the on-Cerbo controller: it reads Venus telemetry
// over D-Bus, drives cooling actuators, and serves the embedded web UI.
//
// v0 scope: connect to D-Bus, read live system metrics, expose them + a relay
// smoke-test in the UI. Control logic (P_avail, MPC, failsafe) lands next.
package main

import (
	"log"

	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/actuator"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/config"
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

	// The Cerbo GX has two on-board relays.
	relays := []actuator.Actuator{
		actuator.NewCerboRelay(bus, 0),
		actuator.NewCerboRelay(bus, 1),
	}

	srv := web.NewServer(cfg, bus, relays, version)
	log.Printf("ruuvi-control %s started, UI on :%s", version, cfg.UIPort)
	log.Fatal(srv.Run())
}
