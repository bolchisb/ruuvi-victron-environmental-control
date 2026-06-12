package venus

// System D-Bus paths we read for live status. Battery, PV and DC system loads
// live on the deterministic com.victronenergy.system service. AC loads and the
// grid connection are read straight from the inverter (VE.Bus), whose service
// name is not fixed and is discovered at read time from /VebusService.
const systemService = "com.victronenergy.system"

// vebusService is a placeholder in Metric.Service: at read time it is replaced
// by the actual VE.Bus inverter service name (e.g. com.victronenergy.vebus.ttyS4),
// which com.victronenergy.system publishes at /VebusService.
const vebusService = "@vebus"

// Metric identifies one telemetry value with its source and unit.
type Metric struct {
	Key     string
	Service string
	Path    string
	Unit    string
}

// SystemMetrics is the v0 set read for the status page. The inverter publishes
// the phase totals at /Ac/Out/P and /Ac/ActiveIn/P (already summed over L1-L3),
// so a single read is correct for both single-phase and three-phase systems.
var SystemMetrics = []Metric{
	{Key: "soc", Service: systemService, Path: "/Dc/Battery/Soc", Unit: "%"},
	{Key: "battery_voltage", Service: systemService, Path: "/Dc/Battery/Voltage", Unit: "V"},
	{Key: "battery_power", Service: systemService, Path: "/Dc/Battery/Power", Unit: "W"},
	{Key: "pv_power", Service: systemService, Path: "/Dc/Pv/Power", Unit: "W"},
	{Key: "ac_loads", Service: vebusService, Path: "/Ac/Out/P", Unit: "W"},
	{Key: "grid", Service: vebusService, Path: "/Ac/ActiveIn/P", Unit: "W"},
	{Key: "dc_loads", Service: systemService, Path: "/Dc/System/Power", Unit: "W"},
}

// Reading is a single metric value or the error that prevented reading it.
type Reading struct {
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"`
	Error string   `json:"error,omitempty"`
}

// ReadSystem reads all SystemMetrics. A failing path yields a Reading with an
// error rather than aborting the whole snapshot. Metrics sourced from the
// inverter are skipped with an error when no VE.Bus inverter is present.
func (b *Bus) ReadSystem() map[string]Reading {
	vebus, vebusErr := b.GetString(systemService, "/VebusService")
	out := make(map[string]Reading, len(SystemMetrics))
	for _, m := range SystemMetrics {
		r := Reading{Unit: m.Unit}
		service := m.Service
		if service == vebusService {
			if vebusErr != nil || vebus == "" {
				r.Error = "no VE.Bus inverter"
				out[m.Key] = r
				continue
			}
			service = vebus
		}
		if v, err := b.GetFloat(service, m.Path); err != nil {
			r.Error = err.Error()
		} else {
			r.Value = &v
		}
		out[m.Key] = r
	}
	return out
}
