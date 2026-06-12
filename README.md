# ruuvi-victron-environmental-control

Predictive climate control for a Victron ESS equipment room that keeps inverter
internal temperature below the thermal-derating threshold while spending minimum
energy on cooling, using Ruuvi sensors and Cerbo GX / Venus OS.

The controller runs directly on the Cerbo GX. It reads system telemetry over
D-Bus, drives cooling outputs (Cerbo relays, GX IO-Extender, Shelly or Modbus),
and serves a small configuration and status web UI styled to match the Victron
GUI.

## Requirements

- A Cerbo GX (or other GX device) running Venus OS.
- Root access enabled on the GX (Settings -> General -> set a root password,
  and enable SSH on LAN).
- [SetupHelper](https://github.com/kwindrem/SetupHelper) installed. It keeps the
  package installed across reboots and reinstalls it automatically after a Venus
  OS firmware update.

## Install on the Cerbo

Enable root access and SSH on the GX, connect, and run the installer. It checks
prerequisites, installs SetupHelper if missing, picks the package for the device
architecture (ARMv7 for Cerbo GX MK1/MK2, ARM64 for aarch64 GX devices),
downloads it and registers the service:

```
ssh root@<cerbo-ip>
wget -qO- https://raw.githubusercontent.com/bolchisb/ruuvi-victron-environmental-control/main/scripts/install.sh | sh
```

By default this installs the latest release. To install a specific release
(including a pre-release), set `TAG` on the shell that runs the script:

```
wget -qO- https://raw.githubusercontent.com/bolchisb/ruuvi-victron-environmental-control/main/scripts/install.sh | TAG=v.0.1.0-dev1 sh
```

When it finishes, open the UI:

```
http://<cerbo-ip>:8088
```

The service runs under daemontools as `/service/ruuvi-control` and is reinstalled
automatically after a firmware update via SetupHelper. The listening port
(`UI_PORT`) and the path of the persisted stage settings (`CONFIG_PATH`, kept
under `/data` so it survives firmware updates) can be changed in
`/data/ruuvi-victron-control/services/ruuvi-control/run`.

If the GX has no internet access, download the matching
`ruuvi-victron-control-<arch>.tgz` from the releases page onto a USB stick,
extract it into `/data`, and run `/data/ruuvi-victron-control/setup`.

### Removing it

Run `/data/ruuvi-victron-control/setup` and choose "Uninstall", or use the
SetupHelper package manager in the GUI.

## Build from source

The binary is cross-compiled inside Docker for both supported GX architectures
(ARMv7 and ARM64). It is never built on the host.

```
scripts/build.sh            # build and package both architectures into ./dist
scripts/build.sh --publish  # also create/upload the GitHub release
scripts/build.sh mac        # build a local binary to test on this machine
```

The version comes from the `VERSION` environment variable, or the `version` file
if it is not set. `--publish` requires the `gh` CLI to be authenticated.

Releases are produced automatically: pushing a tag runs the release workflow,
which builds both architectures and publishes a release named after the tag.

### Testing locally

`scripts/build.sh mac` cross-compiles a binary for this machine in Docker. Run
it with `./dist/mac/ruuvi-control` and open `http://localhost:8088`. There is no
system bus off-device, so the UI reports the bus as unavailable and metrics show
as not available, but the UI, configuration and HTTP API can be exercised.

Before building, drop `Roboto-Regular.ttf` into
`internal/web/static/` so the UI serves the Victron font offline (see the note
in that directory).

## Changelog

### v0.1.0

- Initial controller skeleton that connects to the Venus OS system bus over
  D-Bus and reads live battery state of charge, voltage and power, PV power and
  DC system loads. AC loads and the grid connection are read straight from the
  inverter (VE.Bus, discovered from the system service) using the phase totals,
  so the figures are correct on both single-phase and three-phase systems.
- Temperature sensor discovery: enumerates the temperature services on the bus
  and reads temperature, humidity and pressure for each, plus CO2, VOC, NOX and
  PM2.5 from Ruuvi Air sensors, shown in the UI. Fields a sensor does not report
  show as not available.
- Pluggable output abstraction with the Cerbo on-board relays as the first
  backend.
- Two cooling stages, each with a custom name, an enable switch and a
  temperature setpoint, configured from the UI and persisted as JSON under
  `/data`. Stage 1 switches relay 1 and stage 2 switches relay 2.
- Staged cooling loop: it reads the warmest sensor and switches each enabled
  stage with a hysteresis deadband. Stage 1 (the cheaper output) engages first;
  stage 2 only engages when the room climbs past its higher setpoint, so the
  expensive output runs only when stage 1 cannot hold the temperature.
- Optional air-quality alarm: when a Ruuvi Air reports CO2 or NOX over the
  configured limit, the controller forces stage 1 (ventilation) on and raises an
  alarm shown in the UI.
- Embedded web UI styled to match the Victron GUI: an overview with a battery
  state-of-charge ring showing voltage and power, flanked by solar input and the
  grid connection on the left and AC and DC loads on the right, the temperature
  sensors, and a stages panel where each stage is
  named, enabled or disabled, and has a manual On/Off relay test that reflects
  the live relay state. Light and dark themes with a toggle that is remembered
  between visits.
- The controller starts and serves the UI even when the system bus is
  unavailable, so it can run off-device for testing; the UI shows the bus state.
- Cross-build in Docker for ARMv7 and ARM64, packaged into one install archive
  per architecture.
- One-line installer that detects the device architecture, installs the
  SetupHelper prerequisite and registers the service. On an upgrade it stops the
  running service before replacing the binary and starts it again on the new
  code, so no manual restart is needed.
- Tag-triggered release workflow that builds both architectures and publishes a
  release named after the tag.
- SetupHelper packaging so the service installs under `/data` and survives
  reboots and firmware updates.
