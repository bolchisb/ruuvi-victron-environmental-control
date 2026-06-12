// Package control runs the cooling loop. On a fixed tick it reads temperature
// and air quality from the bus, decides which stages to switch and drives the
// relays.
//
// Cooling is staged through the per-stage setpoints: stage 1 (cheap, e.g.
// exhaust fans) has the lower setpoint and engages first; stage 2 (expensive,
// e.g. AC) has a higher setpoint and only engages when the room climbs past it,
// i.e. when stage 1 cannot hold the temperature. A deadband keeps each stage
// from chattering.
//
// On top of cooling, an air-quality override forces stage 1 (ventilation) on
// and raises an alarm whenever a Ruuvi Air sensor reports CO2 or NOX over the
// configured limit.
package control

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/actuator"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/settings"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/venus"
)

// interval is the control period. Room thermal mass is slow, so a coarse tick
// is plenty and keeps relay wear down.
const interval = 30 * time.Second

// Controller drives the cooling stages from sensor readings.
type Controller struct {
	bus    *venus.Bus
	relays []actuator.Actuator
	store  *settings.Store

	// state is the last commanded on/off per stage, used to hold inside the
	// deadband. Only the control goroutine touches it.
	state []bool

	// airAlarm is read by the web handler from another goroutine.
	airAlarm atomic.Bool
}

// New builds a Controller for the given relays.
func New(bus *venus.Bus, relays []actuator.Actuator, store *settings.Store) *Controller {
	return &Controller{
		bus:    bus,
		relays: relays,
		store:  store,
		state:  make([]bool, len(relays)),
	}
}

// Run executes the control loop until the process exits.
func (c *Controller) Run() {
	c.step()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		c.step()
	}
}

// AirAlarm reports whether the air-quality limit is currently exceeded.
func (c *Controller) AirAlarm() bool {
	return c.airAlarm.Load()
}

func (c *Controller) step() {
	sensors, _ := c.bus.ReadSensors()
	cfg := c.store.Get()

	temp, hasTemp := maxField(sensors, func(s venus.Sensor) *float64 { return s.Temperature })
	alarm := airBreach(sensors, cfg)
	c.setAlarm(alarm)

	for i := range c.relays {
		if i >= len(cfg.Stages) {
			break
		}
		st := cfg.Stages[i]
		desired := c.state[i]

		switch {
		case !st.Enabled:
			// A disabled stage is always off; manual relay tests aside, the
			// controller does not run an output the user turned off.
			desired = false
		case hasTemp:
			if temp >= st.Setpoint {
				desired = true
			} else if temp <= st.Setpoint-cfg.Deadband {
				desired = false
			}
			// Inside the deadband: hold the previous state.
		}

		// Air-quality override: ventilate through stage 1. Only when stage 1 is
		// enabled (otherwise there is nothing wired to run); the alarm itself
		// still fires regardless.
		if alarm && i == 0 && st.Enabled {
			desired = true
		}

		c.apply(i, desired)
	}
}

func (c *Controller) apply(i int, on bool) {
	if err := c.relays[i].Set(on); err != nil {
		// No bus or no relay: keep the intended state for the deadband and log.
		log.Printf("control: set %s=%v: %v", c.relays[i].Name(), on, err)
	}
	c.state[i] = on
}

func (c *Controller) setAlarm(on bool) {
	if c.airAlarm.Swap(on) == on {
		return
	}
	if on {
		log.Printf("control: air quality alarm raised, CO2/NOX over limit")
	} else {
		log.Printf("control: air quality alarm cleared")
	}
}

// airBreach reports whether any sensor's CO2 or NOX exceeds the configured
// limit. Disabled or unconfigured limits never breach.
func airBreach(sensors []venus.Sensor, cfg settings.Settings) bool {
	if !cfg.Air.Enabled {
		return false
	}
	if co2, ok := maxField(sensors, func(s venus.Sensor) *float64 { return s.CO2 }); ok && co2 > cfg.Air.CO2Limit {
		return true
	}
	if nox, ok := maxField(sensors, func(s venus.Sensor) *float64 { return s.NOX }); ok && nox > cfg.Air.NOXLimit {
		return true
	}
	return false
}

// maxField returns the largest non-nil value of a field across all sensors.
// Using the maximum is deliberately conservative: it reacts to the hottest or
// most polluted spot in the room rather than an average that hides it.
func maxField(sensors []venus.Sensor, get func(venus.Sensor) *float64) (float64, bool) {
	max := 0.0
	found := false
	for _, s := range sensors {
		if v := get(s); v != nil {
			if !found || *v > max {
				max = *v
				found = true
			}
		}
	}
	return max, found
}
