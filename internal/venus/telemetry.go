package venus

// System D-Bus paths we read for live status. All live on the deterministic
// com.victronenergy.system service. Verified against the Venus dbus wiki.
const systemService = "com.victronenergy.system"

// Metric identifies one telemetry value with its source and unit.
type Metric struct {
	Key     string
	Service string
	Path    string
	Unit    string
}

// SystemMetrics is the v0 set read for the status page.
var SystemMetrics = []Metric{
	{Key: "soc", Service: systemService, Path: "/Dc/Battery/Soc", Unit: "%"},
	{Key: "pv_power", Service: systemService, Path: "/Dc/Pv/Power", Unit: "W"},
	{Key: "ac_consumption", Service: systemService, Path: "/Ac/ConsumptionOnInput/L1/Power", Unit: "W"},
}

// Reading is a single metric value or the error that prevented reading it.
type Reading struct {
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"`
	Error string   `json:"error,omitempty"`
}

// ReadSystem reads all SystemMetrics. A failing path yields a Reading with an
// error rather than aborting the whole snapshot.
func (b *Bus) ReadSystem() map[string]Reading {
	out := make(map[string]Reading, len(SystemMetrics))
	for _, m := range SystemMetrics {
		r := Reading{Unit: m.Unit}
		if v, err := b.GetFloat(m.Service, m.Path); err != nil {
			r.Error = err.Error()
		} else {
			r.Value = &v
		}
		out[m.Key] = r
	}
	return out
}
