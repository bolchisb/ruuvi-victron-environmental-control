"use strict";

const conn = document.getElementById("conn");
const sensorList = document.getElementById("sensor-list");
const stageList = document.getElementById("stage-list");
const stageSave = document.getElementById("stage-save");
const stageStatus = document.getElementById("stage-status");
const versionEl = document.getElementById("version");
const themeToggle = document.getElementById("theme-toggle");

const brief = document.querySelector(".brief");
const ringValue = document.getElementById("ring-value");
const socVal = document.getElementById("soc-val");
const batteryDetail = document.getElementById("battery-detail");
const pvVal = document.getElementById("pv-val");
const gridVal = document.getElementById("grid-val");
const acVal = document.getElementById("ac-val");
const dcVal = document.getElementById("dc-val");
const pvArc = document.getElementById("pv-arc");
const gridArc = document.getElementById("grid-arc");
const acArc = document.getElementById("ac-arc");
const dcArc = document.getElementById("dc-arc");

function fmt(value) {
  if (value === null || value === undefined) return null;
  return Math.abs(value) >= 100 ? Math.round(value).toString() : value.toFixed(1);
}

// fmtW formats power as whole watts: decimals add width and Victron reports
// power in whole watts anyway.
function fmtW(value) {
  if (value === null || value === undefined) return null;
  return Math.round(value).toString();
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"]/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
}

function reading(system, key) {
  return (system && system[key]) || {};
}

function setText(node, value) {
  node.textContent = value === null ? "–" : value;
}

function renderHero(system, peaks) {
  const soc = reading(system, "soc").value;
  setText(socVal, soc === null || soc === undefined ? null : Math.round(soc).toString());
  const pct = Math.max(0, Math.min(100, soc || 0));
  ringValue.setAttribute("stroke-dasharray", `${pct} 100`);

  const v = fmt(reading(system, "battery_voltage").value);
  const w = fmtW(reading(system, "battery_power").value);
  if (v === null && w === null) {
    batteryDetail.textContent = "–";
  } else {
    batteryDetail.textContent = `${v === null ? "–" : v} V · ${w === null ? "–" : w} W`;
  }

  const pv = reading(system, "pv_power").value;
  const grid = reading(system, "grid").value;
  const ac = reading(system, "ac_loads").value;
  const dc = reading(system, "dc_loads").value;
  setText(pvVal, fmtW(pv));
  setText(gridVal, fmtW(grid));
  setText(acVal, fmtW(ac));
  setText(dcVal, fmtW(dc));

  // Each arc fills against its own remembered peak (tracked and persisted on the
  // device), so a flow is shown relative to its own recent maximum rather than
  // the single largest live flow.
  setArc(pvArc, pv, peaks.pv_power);
  setArc(gridArc, grid, peaks.grid);
  setArc(acArc, ac, peaks.ac_loads);
  setArc(dcArc, dc, peaks.dc_loads);

  fitHero();
}

// The side values sit near the edges of the 1000-unit viewBox; a long figure
// (big watts, or a fresh metric appearing) can run past the edge and clip. The
// base viewBox is widened symmetrically so every value fits, and it only ever
// grows: once stretched to fit the widest figure seen, it holds that size until
// the page is reloaded (a new login or a reboot), so it never jitters per poll.
const heroBaseWidth = 1000;
let heroPad = 0;
function fitHero() {
  const margin = 14; // breathing room (user units) between text and the edge
  let needed = heroPad;
  ["grid-val", "pv-val", "dc-val", "ac-val"].forEach((id) => {
    const text = document.getElementById(id).parentNode; // the <text> element
    let box;
    try {
      box = text.getBBox();
    } catch (e) {
      return; // not laid out yet
    }
    const overLeft = -box.x;
    const overRight = box.x + box.width - heroBaseWidth;
    needed = Math.max(needed, overLeft + margin, overRight + margin);
  });
  if (needed > heroPad) {
    heroPad = Math.ceil(needed);
    brief.setAttribute("viewBox", `${-heroPad} 0 ${heroBaseWidth + 2 * heroPad} 520`);
  }
}

function setArc(node, value, peak) {
  const pct =
    value === null || value === undefined || !(peak > 0)
      ? 0
      : Math.min(100, (Math.abs(value) / peak) * 100);
  node.setAttribute("stroke-dasharray", `${pct} 100`);
}

function metricCell(label, value, unit) {
  const v = fmt(value);
  return (
    `<div class="metric"><span class="label">${label}</span>` +
    (v === null
      ? `<span class="value na">n/a</span>`
      : `<span class="value">${v}<span class="unit">${unit}</span></span>`) +
    `</div>`
  );
}

function renderSensors(sensors) {
  sensorList.innerHTML = "";
  if (!sensors.length) {
    sensorList.innerHTML = `<p class="hint">No temperature sensors detected.</p>`;
    return;
  }
  sensors.forEach((s) => {
    const block = document.createElement("div");
    block.className = "sensor";
    block.innerHTML =
      `<div class="sensor-name">${escapeHtml(s.name)}</div>` +
      `<div class="metric-grid">` +
      metricCell("Temperature", s.temperature, "°C") +
      metricCell("Humidity", s.humidity, "%") +
      metricCell("Pressure", s.pressure, "hPa") +
      metricCell("CO₂", s.co2, "ppm") +
      metricCell("VOC", s.voc, "") +
      metricCell("NOx", s.nox, "") +
      metricCell("PM2.5", s.pm25, "µg/m³") +
      `</div>`;
    sensorList.appendChild(block);
  });
}

// infoTip renders a themed, keyboard-reachable info icon with a tooltip.
function infoTip(text) {
  const t = escapeHtml(text);
  return (
    `<span class="info" tabindex="0" role="img" aria-label="${t}">` +
    `<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/>` +
    `<line x1="12" y1="11" x2="12" y2="16"/><circle cx="12" cy="8" r="1"/></svg>` +
    `<span class="tip" role="tooltip">${t}</span>` +
    `</span>`
  );
}

function fieldLabel(forId, text, tip) {
  return (
    `<span class="field-row">` +
    `<label class="field-label" for="${forId}">${text}</label>` +
    (tip ? infoTip(tip) : "") +
    `</span>`
  );
}

function buildStages(cfg) {
  const stages = cfg.stages || [];
  const defaults = cfg.stageDefaults || [];
  const threshold = cfg.deratingThresholdC;
  const setTip =
    `Victron inverters deliver full output up to ${threshold} °C ambient and ` +
    `begin derating above it. Each stage runs on its default start temperature; ` +
    `use Override to set your own.`;
  stageList.innerHTML = "";
  stages.forEach((st, index) => {
    const def = defaults[index];
    const row = document.createElement("div");
    row.className = "stage";
    row.innerHTML =
      `<div class="stage-head">` +
      `<span class="stage-tag">Stage ${index + 1}</span>` +
      `<label class="stage-enable">` +
      `<input type="checkbox" id="stage-en-${index}"${st.enabled ? " checked" : ""}>` +
      `<span>Enabled</span>` +
      `</label>` +
      `</div>` +
      `<label class="field-label" for="stage-name-${index}">Name</label>` +
      `<input class="stage-name" id="stage-name-${index}" type="text" maxlength="40" value="${escapeHtml(st.name)}">` +
      fieldLabel(`stage-set-${index}`, "Start temperature (°C)", setTip) +
      `<div class="stage-set-row">` +
      `<input class="stage-name stage-set" id="stage-set-${index}" type="number" min="0" step="0.5"` +
      ` value="${st.setpoint}" data-default="${def}"${st.override ? "" : " disabled"}>` +
      `<button type="button" class="override-btn${st.override ? " active" : ""}"` +
      ` id="stage-ovr-${index}" data-i="${index}" aria-pressed="${st.override ? "true" : "false"}">Override</button>` +
      `</div>` +
      `<div class="stage-relay">` +
      `<span class="stage-relay-label">Relay ${index + 1}</span>` +
      `<span class="toggle">` +
      `<button data-i="${index}" data-s="1">On</button>` +
      `<button data-i="${index}" data-s="0">Off</button>` +
      `</span>` +
      `</div>`;
    stageList.appendChild(row);
  });
  stageList.querySelectorAll(".override-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const field = document.getElementById(`stage-set-${btn.dataset.i}`);
      const on = !btn.classList.contains("active");
      btn.classList.toggle("active", on);
      btn.setAttribute("aria-pressed", on ? "true" : "false");
      field.disabled = !on;
      if (on) {
        field.focus();
      } else {
        field.value = field.dataset.default; // back to the built-in default
      }
    });
  });
  stageList.querySelectorAll(".toggle button").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const i = btn.dataset.i, s = btn.dataset.s;
      const group = btn.parentElement.querySelectorAll("button");
      group.forEach((b) => (b.disabled = true));
      try {
        await fetch(`/api/relay?index=${i}&state=${s}`, { method: "POST" });
        group.forEach((b) => b.classList.remove("active"));
        btn.classList.add("active"); // optimistic; next poll confirms
      } finally {
        group.forEach((b) => (b.disabled = false));
      }
    });
  });
}

function renderRelays(outputs) {
  outputs.forEach((o, index) => {
    const buttons = stageList.querySelectorAll(`button[data-i="${index}"]`);
    buttons.forEach((b) => {
      const isOn = b.dataset.s === "1";
      // o.on is null when the real state is unknown (off-bus): clear both.
      b.classList.toggle("active", o.on === true ? isOn : o.on === false ? !isOn : false);
    });
  });
}

function setSaveStatus(text, kind) {
  stageStatus.textContent = text;
  stageStatus.className = "save-status" + (kind ? " save-status--" + kind : "");
}

function renderControlSettings(cfg) {
  const air = cfg.air || {};
  const deadTip =
    "How far the temperature must fall below a stage's start temperature " +
    "before that stage switches off. A wider band stops the stage cycling on " +
    "and off around the setpoint.";
  document.getElementById("control-settings").innerHTML =
    `<div class="stage">` +
    `<span class="stage-tag">Cooling</span>` +
    `<div class="setting">` +
    fieldLabel("deadband", "Deadband (°C)", deadTip) +
    `<input class="stage-name" id="deadband" type="number" min="0.1" step="0.1" value="${cfg.deadband}">` +
    `</div>` +
    `</div>` +
    `<div class="stage">` +
    `<span class="stage-tag">Air quality</span>` +
    `<p class="setting-note">When a Ruuvi Air reports CO₂ or NOx over the limit, ` +
    `stage 1 (exhaust) is forced on to evacuate the gas and an alarm is shown, ` +
    `even if stage 1 cooling is off.</p>` +
    `<label class="stage-enable">` +
    `<input type="checkbox" id="air-enabled"${air.enabled ? " checked" : ""}>` +
    `<span>Air quality alarm</span>` +
    `</label>` +
    `<div class="setting">` +
    fieldLabel("co2-limit", "CO₂ limit (ppm)") +
    `<input class="stage-name" id="co2-limit" type="number" min="0" step="50" value="${air.co2Limit}">` +
    `</div>` +
    `<div class="setting">` +
    fieldLabel("nox-limit", "NOx limit") +
    `<input class="stage-name" id="nox-limit" type="number" min="0" step="1" value="${air.noxLimit}">` +
    `</div>` +
    `</div>`;
}

function applyConfig(cfg) {
  buildStages(cfg);
  renderControlSettings(cfg);
}

async function loadStages() {
  try {
    const res = await fetch("/api/config");
    if (!res.ok) throw new Error(res.statusText);
    applyConfig(await res.json());
  } catch (e) {
    stageList.innerHTML = `<p class="hint">Configuration unavailable.</p>`;
  }
}

function numValue(id) {
  return parseFloat(document.getElementById(id).value) || 0;
}

async function saveStages() {
  const rows = stageList.querySelectorAll(".stage");
  const stages = [];
  rows.forEach((row, index) => {
    stages.push({
      name: document.getElementById(`stage-name-${index}`).value,
      enabled: document.getElementById(`stage-en-${index}`).checked,
      override: document.getElementById(`stage-ovr-${index}`).classList.contains("active"),
      setpoint: numValue(`stage-set-${index}`),
    });
  });
  const payload = {
    stages,
    deadband: numValue("deadband"),
    air: {
      enabled: document.getElementById("air-enabled").checked,
      co2Limit: numValue("co2-limit"),
      noxLimit: numValue("nox-limit"),
    },
  };
  stageSave.disabled = true;
  setSaveStatus("Saving…", "");
  try {
    const res = await fetch("/api/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) throw new Error(res.statusText);
    applyConfig(await res.json());
    setSaveStatus("Saved", "ok");
  } catch (e) {
    setSaveStatus("Save failed", "error");
  } finally {
    stageSave.disabled = false;
  }
}

async function poll() {
  try {
    const res = await fetch("/api/status");
    if (!res.ok) throw new Error(res.statusText);
    const data = await res.json();
    if (data.busConnected) {
      conn.textContent = "online";
      conn.className = "pill pill--on";
    } else {
      conn.textContent = "no d-bus";
      conn.className = "pill pill--error";
    }
    renderHero(data.system || {}, data.peaks || {});
    renderSensors(data.sensors || []);
    renderRelays(data.outputs || []);
    document.getElementById("alarm").hidden = !data.airAlarm;
    versionEl.textContent = "ruuvi-control " + (data.version || "");
  } catch (e) {
    conn.textContent = "offline";
    conn.className = "pill pill--error";
  }
}

function initTheme() {
  const stored = localStorage.getItem("theme");
  const theme = stored === "dark" ? "dark" : "light";
  applyTheme(theme);
  themeToggle.addEventListener("click", () => {
    const next = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    applyTheme(next);
    localStorage.setItem("theme", next);
  });
}

function applyTheme(theme) {
  if (theme === "dark") {
    document.documentElement.setAttribute("data-theme", "dark");
    themeToggle.textContent = "Light";
  } else {
    document.documentElement.removeAttribute("data-theme");
    themeToggle.textContent = "Dark";
  }
}

initTheme();
loadStages();
stageSave.addEventListener("click", saveStages);
poll();
setInterval(poll, 2000);
