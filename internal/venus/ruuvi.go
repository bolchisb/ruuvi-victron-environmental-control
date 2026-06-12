package venus

import "strings"

// Each temperature sensor (Ruuvi or a wired GX input) registers its own
// com.victronenergy.temperature.<id> service. Ruuvi tags additionally expose
// humidity and pressure; wired inputs leave those empty.
const temperaturePrefix = "com.victronenergy.temperature."

// Sensor is one temperature service and its current readings. A missing field
// (e.g. humidity on a wired input) stays nil.
type Sensor struct {
	Service     string   `json:"service"`
	Name        string   `json:"name"`
	Temperature *float64 `json:"temperature"`
	Humidity    *float64 `json:"humidity"`
	Pressure    *float64 `json:"pressure"`
}

// ReadSensors enumerates the temperature services on the bus and reads each
// one. Returns ErrNotConnected (via Names) when there is no bus.
func (b *Bus) ReadSensors() ([]Sensor, error) {
	names, err := b.Names()
	if err != nil {
		return nil, err
	}

	var sensors []Sensor
	for _, name := range names {
		if !strings.HasPrefix(name, temperaturePrefix) {
			continue
		}
		s := Sensor{Service: name, Name: b.sensorName(name)}
		if v, err := b.GetFloat(name, "/Temperature"); err == nil {
			s.Temperature = &v
		}
		if v, err := b.GetFloat(name, "/Humidity"); err == nil {
			s.Humidity = &v
		}
		if v, err := b.GetFloat(name, "/Pressure"); err == nil {
			s.Pressure = &v
		}
		sensors = append(sensors, s)
	}
	return sensors, nil
}

// sensorName prefers the user-set CustomName, falling back to the product name
// and finally the service id.
func (b *Bus) sensorName(service string) string {
	if n, err := b.GetString(service, "/CustomName"); err == nil && n != "" {
		return n
	}
	if n, err := b.GetString(service, "/ProductName"); err == nil && n != "" {
		return n
	}
	return strings.TrimPrefix(service, "com.victronenergy.")
}
