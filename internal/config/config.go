package config

import "os"

// Config holds runtime configuration. Values come from environment variables
// (set in the daemontools `run` script on the Cerbo). No URLs are hardcoded.
type Config struct {
	// UIPort is the TCP port the embedded web UI listens on.
	// 80 = Remote Console, 1881 = Node-RED, 3000 = Signal K are taken on Venus OS.
	UIPort string
}

// Load reads configuration from the environment.
func Load() Config {
	port := os.Getenv("UI_PORT")
	if port == "" {
		port = "8088"
	}
	return Config{UIPort: port}
}
