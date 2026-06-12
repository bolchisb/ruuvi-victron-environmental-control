package actuator

import (
	"fmt"

	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/venus"
)

// CerboRelay drives one of the Cerbo's two on-board relays via D-Bus:
// com.victronenergy.system /Relay/<index>/State (0/1). index is 0-based.
type CerboRelay struct {
	bus   *venus.Bus
	index int
}

// NewCerboRelay builds a relay actuator for the given 0-based relay index.
func NewCerboRelay(bus *venus.Bus, index int) *CerboRelay {
	return &CerboRelay{bus: bus, index: index}
}

// Set writes the relay state.
func (r *CerboRelay) Set(on bool) error {
	var v int32
	if on {
		v = 1
	}
	return r.bus.SetInt("com.victronenergy.system", fmt.Sprintf("/Relay/%d/State", r.index), v)
}

// Name returns a 1-based human label, e.g. "Cerbo Relay 1".
func (r *CerboRelay) Name() string {
	return fmt.Sprintf("Cerbo Relay %d", r.index+1)
}
