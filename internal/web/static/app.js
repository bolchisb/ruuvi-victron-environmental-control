"use strict";

const conn = document.getElementById("conn");
const sensorList = document.getElementById("sensor-list");
const stageList = document.getElementById("stage-list");
const stageSave = document.getElementById("stage-save");
const stageStatus = document.getElementById("stage-status");
const versionEl = document.getElementById("version");
const themeToggle = document.getElementById("theme-toggle");

const ringValue = document.getElementById("ring-value");
const socVal = document.getElementById("soc-val");
const batteryDetail = document.getElementById("battery-detail");
const pvVal = document.getElementById("pv-val");
const gridVal = document.getElementById("grid-val");
const acVal = document.getElementById("ac-val");
const dcVal = document.getElementById("dc-val");

function fmt(value) {
  if (value === null || value === undefined) return null;
  return Math.abs(value) >= 100 ? Math.round(value).toString() : value.toFixed(1);
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

function renderHero(system) {
  const soc = reading(system, "soc").value;
  setText(socVal, soc === null || soc === undefined ? null : Math.round(soc).toString());
  const pct = Math.max(0, Math.min(100, soc || 0));
  ringValue.setAttribute("stroke-dasharray", `${pct} 100`);

  const v = fmt(reading(system, "battery_voltage").value);
  const w = fmt(reading(system, "battery_power").value);
  if (v === null && w === null) {
    batteryDetail.textContent = "–";
  } else {
    batteryDetail.textContent = `${v === null ? "–" : v} V · ${w === null ? "–" : w} W`;
  }

  setText(pvVal, fmt(reading(system, "pv_power").value));
  setText(gridVal, fmt(reading(system, "grid").value));
  setText(acVal, fmt(reading(system, "ac_loads").value));
  setText(dcVal, fmt(reading(system, "dc_loads").value));
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

function buildStages(stages) {
  stageList.innerHTML = "";
  stages.forEach((st, index) => {
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
      `<label class="field-label" for="stage-set-${index}">Start at (°C)</label>` +
      `<input class="stage-name" id="stage-set-${index}" type="number" min="0" step="0.5" value="${st.setpoint}">` +
      `<div class="stage-relay">` +
      `<span class="stage-relay-label">Relay ${index + 1}</span>` +
      `<span class="toggle">` +
      `<button data-i="${index}" data-s="1">On</button>` +
      `<button data-i="${index}" data-s="0">Off</button>` +
      `</span>` +
      `</div>`;
    stageList.appendChild(row);
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
  document.getElementById("control-settings").innerHTML =
    `<div class="setting">` +
    `<label class="field-label" for="deadband">Deadband (°C)</label>` +
    `<input class="stage-name" id="deadband" type="number" min="0.1" step="0.1" value="${cfg.deadband}">` +
    `</div>` +
    `<label class="stage-enable">` +
    `<input type="checkbox" id="air-enabled"${air.enabled ? " checked" : ""}>` +
    `<span>Air quality alarm</span>` +
    `</label>` +
    `<div class="setting">` +
    `<label class="field-label" for="co2-limit">CO₂ limit (ppm)</label>` +
    `<input class="stage-name" id="co2-limit" type="number" min="0" step="50" value="${air.co2Limit}">` +
    `</div>` +
    `<div class="setting">` +
    `<label class="field-label" for="nox-limit">NOx limit</label>` +
    `<input class="stage-name" id="nox-limit" type="number" min="0" step="1" value="${air.noxLimit}">` +
    `</div>`;
}

function applyConfig(cfg) {
  buildStages(cfg.stages || []);
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
    renderHero(data.system || {});
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
