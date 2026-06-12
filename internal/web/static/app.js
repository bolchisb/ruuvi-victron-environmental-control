"use strict";

const conn = document.getElementById("conn");
const sensorList = document.getElementById("sensor-list");
const outputList = document.getElementById("output-list");
const versionEl = document.getElementById("version");
const themeToggle = document.getElementById("theme-toggle");

const ringValue = document.getElementById("ring-value");
const socVal = document.getElementById("soc-val");
const batteryDetail = document.getElementById("battery-detail");
const pvVal = document.getElementById("pv-val");
const acVal = document.getElementById("ac-val");
const dcVal = document.getElementById("dc-val");

let outputsBuilt = false;

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
  setText(acVal, fmt(reading(system, "ac_consumption").value));
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
      `</div>`;
    sensorList.appendChild(block);
  });
}

function buildOutputs(outputs) {
  outputList.innerHTML = "";
  outputs.forEach((o, index) => {
    const row = document.createElement("div");
    row.className = "output";
    row.innerHTML =
      `<span>${escapeHtml(o.name)}</span>` +
      `<span class="toggle">` +
      `<button data-i="${index}" data-s="1">On</button>` +
      `<button data-i="${index}" data-s="0">Off</button>` +
      `</span>`;
    outputList.appendChild(row);
  });
  outputList.querySelectorAll("button").forEach((btn) => {
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
  outputsBuilt = true;
}

function renderOutputs(outputs) {
  if (!outputsBuilt) buildOutputs(outputs);
  outputs.forEach((o, index) => {
    const buttons = outputList.querySelectorAll(`button[data-i="${index}"]`);
    buttons.forEach((b) => {
      const isOn = b.dataset.s === "1";
      // o.on is null when the real state is unknown (off-bus): clear both.
      b.classList.toggle("active", o.on === true ? isOn : o.on === false ? !isOn : false);
    });
  });
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
    renderOutputs(data.outputs || []);
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
poll();
setInterval(poll, 2000);
