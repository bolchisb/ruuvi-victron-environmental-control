// Package actuator abstracts the switched outputs that drive cooling stages.
//
// Planned backends: Cerbo on-board relays, GX IO-Extender, Shelly, Modbus.
// Each cooling stage maps to one Actuator. The thermostat failsafe drives
// actuators through this same interface, so safety works regardless of which
// backend the user selects.
package actuator

// Actuator is a single switched output.
type Actuator interface {
	// Set turns the output on or off.
	Set(on bool) error
	// State reports whether the output is currently on.
	State() (bool, error)
	// Name is a human-readable label shown in the UI.
	Name() string
}
