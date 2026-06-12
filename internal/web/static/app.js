"use strict";

const LABELS = {
  soc: "Battery SOC",
  pv_power: "PV Power",
  ac_consumption: "AC Consumption",
};

const conn = document.getElementById("conn");
const metricGrid = document.getElementById("metric-grid");
const outputList = document.getElementById("output-list");
const versionEl = document.getElementById("version");

let outputsRendered = false;

function fmt(value) {
  if (value === null || value === undefined) return null;
  return Math.abs(value) >= 100 ? Math.round(value).toString() : value.toFixed(1);
}

function renderMetrics(system) {
  metricGrid.innerHTML = "";
  for (const key of Object.keys(LABELS)) {
    const r = system[key] || {};
    const v = fmt(r.value);
    const cell = document.createElement("div");
    cell.className = "metric";
    cell.innerHTML =
      `<span class="label">${LABELS[key]}</span>` +
      (v === null
        ? `<span class="value na">n/a</span>`
        : `<span class="value">${v}<span class="unit">${r.unit || ""}</span></span>`);
    metricGrid.appendChild(cell);
  }
}

function renderOutputs(outputs) {
  if (outputsRendered) return; // static for v0
  outputList.innerHTML = "";
  outputs.forEach((name, index) => {
    const row = document.createElement("div");
    row.className = "output";
    row.innerHTML =
      `<span>${name}</span>` +
      `<span class="toggle">` +
      `<button data-i="${index}" data-s="1">On</button>` +
      `<button data-i="${index}" data-s="0">Off</button>` +
      `</span>`;
    outputList.appendChild(row);
  });
  outputList.querySelectorAll("button").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const i = btn.dataset.i, s = btn.dataset.s;
      await fetch(`/api/relay?index=${i}&state=${s}`, { method: "POST" });
      const siblings = btn.parentElement.querySelectorAll("button");
      siblings.forEach((b) => b.classList.remove("active"));
      btn.classList.add("active");
    });
  });
  outputsRendered = true;
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
    renderMetrics(data.system || {});
    renderOutputs(data.outputs || []);
    versionEl.textContent = "ruuvi-control " + (data.version || "");
  } catch (e) {
    conn.textContent = "offline";
    conn.className = "pill pill--error";
  }
}

poll();
setInterval(poll, 2000);
