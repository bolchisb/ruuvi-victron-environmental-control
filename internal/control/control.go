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
// On top of cooling, an air-quality override forces stage 1 (ventilation) on to
// evacuate the gas — regardless of whether stage 1 cooling is enabled — and
// raises an alarm whenever a Ruuvi Air sensor reports CO2 or NOX over the
// configured limit.
//
// A thermal-derating override sits above the energy gating: when the inverter
// reports it is derating — its high-temperature alarm is raised or its available
// output rating has dropped below the learned maximum — stage 1 is forced on
// immediately to pull the heat out, ahead of the room setpoint and regardless of
// the energy situation. If the room does not fall within the escalation window,
// stage 2 is brought in as well.
//
// Energy-aware gating always applies: a stage only runs while there is enough
// solar surplus (PV power minus loads) to cover it and the battery is above the SoC
// floor: a small surplus permits stage 1, a larger one permits stage 2. Below
// that the controller will not cool from the grid until the room reaches the
// grid-cooling temperature while the inverter has been under sustained heavy load
// (its AC output above its own rolling 24-hour average for the sustain window) —
// a hot reading at light load is not worth grid energy, since the inverter only
// derates under load. When both hold, hardware protection overrides cost and every
// stage is permitted. Gas evacuation always runs, including on the grid.
package control

import (
	"log"
	"math"
	"sync/atomic"
	"time"

	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/actuator"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/loadavg"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/settings"
	"github.com/bolchisb/ruuvi-victron-environmental-control/internal/venus"
)

// interval is the control period. Room thermal mass is slow, so a coarse tick
// is plenty and keeps relay wear down.
const interval = 30 * time.Second

// surplusHysteresisW is how far the solar surplus may dip below a stage's
// threshold before energy gating drops that stage. A running stage adds its own
// draw to the load, cutting the surplus; the margin stops it from immediately
// gating itself back off.
const surplusHysteresisW = 150

// loadHysteresisW is how far the inverter AC output may dip below the rolling
// average before the sustained-load counter resets. A brief dip should not throw
// away minutes of accumulated high load.
const loadHysteresisW = 200

// deratingDropFraction is how far the inverter's available output rating may fall
// below its learned maximum before that drop counts as thermal derating. A small
// margin keeps normal measurement noise from tripping it.
const deratingDropFraction = 0.05

// escalateTicks is how long stage 1 is given to pull the room down once derating
// starts before stage 2 is brought in. At the 30 s tick this is about three
// minutes.
const escalateTicks = 6

// tempFallTolerance is how much the room must drop from where derating began for
// stage 1 to count as winning; below that fall, derating escalates to stage 2.
const tempFallTolerance = 0.3

// Controller drives the cooling stages from sensor readings.
type Controller struct {
	bus     *venus.Bus
	relays  []actuator.Actuator
	store   *settings.Store
	loadAvg *loadavg.Store

	// state is the last commanded on/off per stage, used to hold inside the
	// deadband. Only the control goroutine touches it.
	state []bool

	// loadHighTicks counts consecutive ticks the inverter AC output has stayed at
	// or above its rolling 24-hour average, so the grid-cooling override fires only
	// on sustained load. Only the control goroutine touches it.
	loadHighTicks int

	// Thermal-derating tracking, touched only by the control goroutine.
	// deratingBaselineW is the largest available output rating seen, used as the
	// "not derating" reference; deratingTicks counts how long derating has been
	// active; deratingStartTemp is the room temperature when it began, against
	// which the trend decides whether to escalate to stage 2.
	deratingBaselineW    float64
	deratingTicks        int
	deratingStartTemp    float64
	hasDeratingStartTemp bool

	// airAlarm and derating are read by the web handler from another goroutine.
	airAlarm atomic.Bool
	derating atomic.Bool
}

// New builds a Controller for the given relays.
func New(bus *venus.Bus, relays []actuator.Actuator, store *settings.Store, loadAvg *loadavg.Store) *Controller {
	return &Controller{
		bus:     bus,
		relays:  relays,
		store:   store,
		loadAvg: loadAvg,
		state:   make([]bool, len(relays)),
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

// Derating reports whether the inverter is currently thermal-derating.
func (c *Controller) Derating() bool {
	return c.derating.Load()
}

func (c *Controller) step() {
	sensors, _ := c.bus.ReadSensors()
	cfg := c.store.Get()
	sys := c.bus.ReadSystem()

	temp, hasTemp := maxField(sensors, func(s venus.Sensor) *float64 { return s.Temperature })
	alarm := airBreach(sensors, cfg)
	c.setAlarm(alarm)

	// permitted is how many stages, in order, the current energy situation lets
	// run this tick.
	c.trackLoad(time.Now(), cfg, sys)
	permitted := c.permittedStages(cfg, sys, temp, hasTemp)

	// derating is the inverter reporting thermal derating; escalate is set once
	// stage 1 has had its window without the room falling.
	derating := c.trackDerating(sys, temp, hasTemp)
	escalate := derating && c.deratingEscalate(temp, hasTemp)

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
		case i >= permitted:
			// Gated out by the energy situation: keep it off until either there
			// is enough surplus or the room reaches the grid-cooling temperature.
			desired = false
		case hasTemp:
			if temp >= st.Setpoint {
				desired = true
			} else if temp <= st.Setpoint-cfg.Deadband {
				desired = false
			}
			// Inside the deadband: hold the previous state.
		}

		// Air-quality override: force stage 1 (exhaust) on to evacuate the gas.
		// This is a safety action, so it runs even when stage 1 cooling is
		// disabled or gated out by the energy situation.
		if alarm && i == 0 {
			desired = true
		}

		// Thermal-derating override: the inverter is throttling on heat, so pull
		// it back ahead of the room setpoint and the energy gating — protecting the
		// inverter beats saving energy. Stage 1 goes on immediately; stage 2 joins
		// only once stage 1 has had its window without the room falling.
		if derating && i == 0 {
			desired = true
		}
		if escalate && i == 1 {
			desired = true
		}

		c.apply(i, desired)
	}
}

// permittedStages returns how many stages (in order) the current energy
// situation allows to run. When the telemetry it needs is unavailable, every
// stage is permitted and temperature alone decides — a missing reading must
// never block cooling.
func (c *Controller) permittedStages(cfg settings.Settings, sys map[string]venus.Reading, temp float64, hasTemp bool) int {
	all := len(c.relays)
	// Hardware protection wins over cost, but only when heat is actually being
	// made: above the grid-cooling temperature and with the inverter under
	// sustained heavy load, cool from any source, grid included. A hot room at low
	// load is not worth grid energy — the inverter only derates under load.
	if hasTemp && temp >= cfg.Energy.GridCoolTemp && c.loadSustained(cfg) {
		return all
	}

	pv, okPV := sysValue(sys, "pv_power")
	ac, okAC := sysValue(sys, "ac_loads")
	dc, okDC := sysValue(sys, "dc_loads")
	soc, okSoC := sysValue(sys, "soc")
	if !okPV || !okAC || !okDC || !okSoC {
		return all
	}

	// Do not drain the battery for cooling below the floor.
	if soc < cfg.Energy.SocFloor {
		return 0
	}

	surplus := pv - (ac + dc)
	s1 := cfg.Energy.Stage1SurplusW
	s2 := cfg.Energy.Stage2SurplusW
	// A stage that is already running keeps its permission until the surplus
	// drops a margin below its threshold (see surplusHysteresisW).
	if len(c.state) > 0 && c.state[0] {
		s1 -= surplusHysteresisW
	}
	if len(c.state) > 1 && c.state[1] {
		s2 -= surplusHysteresisW
	}

	switch {
	case surplus >= s2:
		return all
	case surplus >= s1:
		return 1
	default:
		return 0
	}
}

// trackLoad feeds the inverter AC output into the rolling 24-hour average and
// updates the sustained-load counter against that average. The counter counts up
// while the output is at or above the average and resets once it falls a margin
// below it, so a brief spike never counts as sustained and a brief dip never
// throws the count away. A missing reading holds the count.
func (c *Controller) trackLoad(now time.Time, cfg settings.Settings, sys map[string]venus.Reading) {
	ac, ok := sysValue(sys, "ac_loads")
	if !ok {
		return
	}
	c.loadAvg.Add(now, ac)
	mean, has := c.loadAvg.Mean()
	if !has {
		return
	}
	switch {
	case ac >= mean:
		if need := loadSustainTicks(cfg); c.loadHighTicks < need {
			c.loadHighTicks++
		}
	case ac < mean-loadHysteresisW:
		c.loadHighTicks = 0
	}
}

// loadSustained reports whether the inverter has held heavy load for the whole
// sustain window.
func (c *Controller) loadSustained(cfg settings.Settings) bool {
	return c.loadHighTicks >= loadSustainTicks(cfg)
}

// loadSustainTicks is the configured sustain window expressed in control ticks,
// rounded up and at least one.
func loadSustainTicks(cfg settings.Settings) int {
	t := int(math.Ceil(cfg.Energy.LoadSustainMin * 60 / interval.Seconds()))
	if t < 1 {
		t = 1
	}
	return t
}

// trackDerating updates the derating state for this tick and reports whether the
// inverter is currently derating. While derating is active it counts ticks and
// remembers the room temperature it started at, so deratingEscalate can tell
// whether stage 1 is holding. When it clears, the counter and start temperature
// reset so the next event measures its own trend.
func (c *Controller) trackDerating(sys map[string]venus.Reading, temp float64, hasTemp bool) bool {
	active := c.deratingDetected(sys)
	c.derating.Store(active)
	if !active {
		c.deratingTicks = 0
		c.hasDeratingStartTemp = false
		return false
	}
	c.deratingTicks++
	if hasTemp && !c.hasDeratingStartTemp {
		c.deratingStartTemp = temp
		c.hasDeratingStartTemp = true
	}
	return true
}

// deratingDetected reports whether the inverter is thermal-derating, from either
// the VE.Bus high-temperature alarm or a drop in the available output rating below
// its learned maximum. The maximum is learned from the highest rating seen, so the
// drop threshold adapts to the unit without any hardcoded wattage. A missing
// reading simply does not trigger.
func (c *Controller) deratingDetected(sys map[string]venus.Reading) bool {
	if alarm, ok := sysValue(sys, "inverter_temp_alarm"); ok && alarm >= 1 {
		return true
	}
	if nom, ok := sysValue(sys, "inverter_nominal_power"); ok && nom > 0 {
		if nom > c.deratingBaselineW {
			c.deratingBaselineW = nom
		}
		if c.deratingBaselineW > 0 && nom < c.deratingBaselineW*(1-deratingDropFraction) {
			return true
		}
	}
	return false
}

// deratingEscalate reports whether derating should pull in stage 2: stage 1 has
// had the escalation window and the room has not fallen a meaningful amount from
// where derating began.
func (c *Controller) deratingEscalate(temp float64, hasTemp bool) bool {
	if !hasTemp || !c.hasDeratingStartTemp || c.deratingTicks < escalateTicks {
		return false
	}
	return temp >= c.deratingStartTemp-tempFallTolerance
}

// sysValue extracts a present numeric reading from a ReadSystem snapshot.
func sysValue(sys map[string]venus.Reading, key string) (float64, bool) {
	r, ok := sys[key]
	if !ok || r.Value == nil {
		return 0, false
	}
	return *r.Value, true
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
