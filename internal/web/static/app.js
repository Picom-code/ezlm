"use strict";

// ---------- plumbing ----------

const token = (() => {
  const t = new URLSearchParams(location.search).get("t");
  try {
    if (t) sessionStorage.setItem("ezlm-token", t);
    return t || sessionStorage.getItem("ezlm-token") || "";
  } catch { return t || ""; }
})();

async function api(path, body) {
  const res = await fetch("/api/" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: { "X-EZLM-Token": token, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || (res.status === 401 ? "This page is out of date. Close it and open EZLM again." : res.statusText));
  return data;
}

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
const view = $("#view");
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const GB = 1024 ** 3;
const gb = (b) => (b / GB).toFixed(b < 10 * GB ? 1 : 0) + " GB";
const ctxLabel = (n) => Math.round(n / 1024) + "k";
const words = (tps) => Math.max(1, Math.round(tps * 0.75));
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const calm = matchMedia("(prefers-reduced-motion: reduce)").matches;

function eta(done, total, bps) {
  if (!bps || !total || done >= total) return "";
  const s = (total - done) / bps;
  if (s < 60) return Math.ceil(s) + "s left";
  if (s < 3600) return Math.ceil(s / 60) + " min left";
  return (s / 3600).toFixed(1) + " h left";
}

// Types text into el one character at a time. Newer calls cancel older ones.
const typing = new WeakMap();
async function type(el, text, speed = 28) {
  if (!el) return;
  const id = Symbol();
  typing.set(el, id);
  if (calm) { el.textContent = text; return; }
  el.textContent = "";
  el.classList.add("caret");
  for (const ch of text) {
    if (typing.get(el) !== id) return;
    el.textContent += ch;
    await sleep(speed);
  }
  el.classList.remove("caret");
}

// Enter presses the screen's primary button (unless focus is on a control).
document.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" || e.metaKey || e.ctrlKey || e.altKey) return;
  const tag = document.activeElement?.tagName;
  if (["BUTTON", "INPUT", "TEXTAREA", "SUMMARY", "A"].includes(tag)) return;
  const btn = $("button.primary:not(:disabled)");
  if (btn) { e.preventDefault(); btn.click(); }
});

// ---------- logos ----------

const LOGO = {
  E: ["███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"],
  Z: ["███████╗", "╚══███╔╝", "  ███╔╝ ", " ███╔╝  ", "███████╗", "╚══════╝"],
  L: ["██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"],
  M: ["███╗   ███╗", "████╗ ████║", "██╔████╔██║", "██║╚██╔╝██║", "██║ ╚═╝ ██║", "╚═╝     ╚═╝"],
};
const logo = $("#logo");
logo.innerHTML = LOGO.E.map((_, i) =>
  `<span class="line" style="animation-delay:${i * 60}ms"><span class="ez">${LOGO.E[i]}${LOGO.Z[i]}</span><span class="lm">${LOGO.L[i]}${LOGO.M[i]}</span></span>`
).join("");
const busy = (on) => logo.classList.toggle("busy", on);
function celebrate() {
  logo.classList.remove("busy", "done");
  void logo.offsetWidth;
  logo.classList.add("done");
}

// 5-row pixel font for the app marks, drawn with half blocks (2 rows per line).
const PIX = {
  o: [".##.", "#..#", "#..#", "#..#", ".##."],
  p: ["###.", "#..#", "###.", "#...", "#..."],
  e: ["####", "#...", "###.", "#...", "####"],
  n: ["#..#", "##.#", "#.##", "#..#", "#..#"],
  c: [".###", "#...", "#...", "#...", ".###"],
  d: ["###.", "#..#", "#..#", "#..#", "###."],
  h: ["#..#", "#..#", "####", "#..#", "#..#"],
  r: ["###.", "#..#", "###.", "#.#.", "#..#"],
  m: ["#...#", "##.##", "#.#.#", "#...#", "#...#"],
  s: [".###", "#...", ".##.", "...#", "###."],
};
function pixelWord(word) {
  const rows = [0, 1, 2, 3, 4, 5].map((r) => [...word].map((ch) => (PIX[ch][r] ?? ".".repeat(PIX[ch][0].length)) + ".").join(""));
  const lines = [];
  for (let r = 0; r < 6; r += 2) {
    let line = "";
    for (let x = 0; x < rows[r].length; x++) {
      const top = rows[r][x] === "#", bot = rows[r + 1][x] === "#";
      line += top && bot ? "█" : top ? "▀" : bot ? "▄" : " ";
    }
    lines.push(line);
  }
  return lines;
}
// The pixel wordmark a tile shows when its logo file is missing. OpenCode is
// grey "open" + white "code"; Hermes is gold; DeepSeek Harness is a white "dsh".
function appMark(id) {
  if (id === "opencode") {
    const a = pixelWord("open"), b = pixelWord("code");
    return `<pre class="mark" aria-hidden="true">${a.map((l, i) => `<span class="a">${l}</span><span class="b">${b[i]}</span>`).join("\n")}</pre>`;
  }
  if (id === "hermes") return `<pre class="mark gold" aria-hidden="true">${pixelWord("hermes").map((l) => `<span class="a">${l}</span>`).join("\n")}</pre>`;
  if (id === "dsh") return `<pre class="mark" aria-hidden="true">${pixelWord("dsh").map((l) => `<span class="b">${l}</span>`).join("\n")}</pre>`;
  return `<span class="mark-name" aria-hidden="true">${esc(id)}</span>`;
}

// An app's logo (/logos/app-<icon>.png) in a square box: 36 on tiles, 20 in
// lists, 16 on buttons. The dsh whale is a bare glyph, drawn at 80 %. Without
// the file: the wordmark on tiles, nothing elsewhere (see the error handler below).
function appLogo(t, size, fallback = false) {
  const key = /^[a-z0-9-]+$/i.test(t.icon || "") ? t.icon.toLowerCase() : t.id;
  return `<span class="applogo${key === "dsh" ? " glyph" : ""}" style="--s:${size}px" aria-hidden="true"${fallback ? ` data-mark="${esc(t.id)}"` : ""}>` +
    `<img src="/logos/app-${esc(key)}.png" alt="" width="${size}" height="${size}" decoding="async"></span>`;
}
document.addEventListener("error", (e) => {
  const img = e.target;
  if (img.tagName !== "IMG" || !img.parentElement?.classList.contains("applogo")) return;
  const box = img.parentElement;
  if (box.dataset.mark) box.outerHTML = appMark(box.dataset.mark);
  else box.remove();
}, true);

// ---------- connected apps (same words as the Mac and Windows apps) ----------

const appCurrent = (t) => t.state || (t.connected ? "connected" : t.installed ? "installed" : "not_installed");

function appStateLine(t, s) {
  if (!t.supported) return { text: t.why || "Not available on this computer", tone: "" };
  // An engine without states: Hermes shows as connected while the model has less than 64K.
  if (!t.state && t.connected && t.id === "hermes" && s?.hermes64) return { text: "Connected, but it needs a 64K context: connect again to give the model room", tone: "warn" };
  const d = t.detail || t.last_check?.msg || "";
  switch (appCurrent(t)) {
    case "connected": return { text: "✓ Connected", tone: "on" };
    case "installed": return { text: "Installed, not connected yet", tone: "" };
    case "needs_update": return { text: "Points at an old address or key: EZLM fixes it when you open it", tone: "warn" };
    case "needs_64k": return { text: d || `${t.name} needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model.`, tone: "warn" };
    case "needs_tools": return { text: d || `This model can't use tools, so ${t.name} can't edit files with it. Pick a model marked Tools, or use Chat.`, tone: "warn" };
    case "broken": {
      const head = t.last_check?.stage === "launch" ? "Couldn't open last time" : "The last check failed";
      return { text: d ? `${head}: ${d}` : head + ".", tone: "err" };
    }
    default: return { text: "Not installed: EZLM will install it", tone: "" };
  }
}

const appVersionLine = (t) => t.version ? `${t.version} · ${t.source === "ezlm" ? "installed by EZLM" : "yours"}` : "";

// "2 min ago", "3 h ago", "yesterday", "Nov 1" (as the apps say it).
function ago(date, now = new Date()) {
  const d = (now - date) / 1000;
  if (d < 60) return "just now";
  if (d < 3600) return `${Math.floor(d / 60)} min ago`;
  if (d < 86400) return `${Math.floor(d / 3600)} h ago`;
  if (d < 172800) return "yesterday";
  return date.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

function appCheckedLine(t) {
  const c = t.last_check;
  const at = c?.at ? new Date(c.at) : null;
  return c?.ok && c.msg && at && !isNaN(at) ? `Checked ${ago(at)}: ${c.msg}` : "";
}

// The buttons that open an app, main one first.
function appModes(t) {
  const l = (t.launch || []).filter((m) => m.label);
  return l.length ? l : [{ id: "tui", label: "Open " + t.name }];
}

// Hermes needs a 64K context: what that costs the running model, if anything.
function hermesTrade(s) {
  const h = s?.hermes64;
  if (!h) return null;
  const name = s.config?.model_name || "your model";
  if (!h.fits) return { text: `Hermes needs a 64K context, which ${name} can't have on this computer. Switch to a smaller model to use it with Hermes.`, warn: true, blocked: true };
  if (h.tok_per_sec >= 0.75 * h.now_tok_per_sec) return { text: "Hermes needs a 64K context; EZLM gives the model the extra room when you connect (same speed).", warn: false, blocked: false };
  return { text: `Hermes needs a 64K context. With it, ${name} would write about ${Math.max(1, Math.floor(h.tok_per_sec * 0.75))} words/s instead of ${Math.max(1, Math.floor(h.now_tok_per_sec * 0.75))}, for every app.`, warn: true, blocked: false };
}

const ATTRIBUTION = "App logos identify the apps EZLM connects to and are trademarks of their owners: OpenCode (Anomaly Innovations, Inc.; square logo from github.com/anomalyco/opencode, MIT), Hermes Agent (Nous Research, Inc.; app icon from github.com/NousResearch/hermes-agent, MIT), DeepSeek Harness (DeepSeek; whale icon from github.com/deepseek-ai/deepseek-harness, MIT). EZLM is compatible with these apps but is not affiliated with or endorsed by their makers.";

// ---------- meters ----------

function meter(cls = "", id = "") {
  return `<div class="meter ${cls}" ${id ? `id="${id}"` : ""}><div class="fill" style="width:0%"></div></div>`;
}
function setMeter(el, frac) {
  if (!el) return;
  el.classList.remove("indet");
  el.querySelector(".fill").style.width = (Math.max(0, Math.min(1, frac)) * 100).toFixed(1) + "%";
}
function indet(el) { el?.classList.add("indet", "live"); }

// A memory bar: what other apps hold, then the model's share. Segments grow in on render.
function memBar(total, others, need) {
  const pct = (v) => ((Math.max(0, v) / total) * 100).toFixed(2) + "%";
  return `<div class="meter" data-grow>
      <div class="seg others" style="width:0" data-w="${pct(others)}"></div>
      <div class="seg" style="left:${pct(others)};width:0" data-w="${pct(Math.min(need, total - others))}"></div>
    </div>`;
}
function growBars(root) {
  requestAnimationFrame(() => requestAnimationFrame(() => {
    $$("[data-grow] .seg", root).forEach((s, i) => setTimeout(() => (s.style.width = s.dataset.w), 60 + i * 50));
    $$("[data-to]", root).forEach((f, i) => setTimeout(() => (f.style.width = f.dataset.to), 60 + i * 50));
  }));
}

// ---------- event stream ----------

let handlers = null; // set by the screen that is listening
function listen() {
  const es = new EventSource("/api/events?t=" + encodeURIComponent(token));
  es.onmessage = (m) => { if (handlers) handlers(JSON.parse(m.data)); };
}

// ---------- boot ----------

let scanResult = null;
let chosenID = null;

async function boot() {
  listen();
  try {
    const s = await api("status");
    if (s.busy === "deploy") return showDeploy(true);
    if (s.deployed) return showStatus(s);
    showScan();
  } catch (e) {
    view.innerHTML = `<p class="error">${esc(e.message)}</p>`;
  }
}

// ---------- scan ----------

const SCAN_LINES = [
  ["cpu", "Processor"],
  ["mem", "System memory"],
  ["gpu", "Graphics"],
  ["disk", "Disk space"],
  ["fit", "Comparing models against your memory"],
];

async function showScan() {
  handlers = null;
  busy(true);
  view.innerHTML = `<ul class="log" id="scanlog">${SCAN_LINES.map(([id, label], i) =>
    `<li id="sc-${id}" class="run reveal" style="animation-delay:${i * 90}ms"><span class="mark spin"></span><span class="text">${label}<span class="sub"></span></span></li>`).join("")}</ul>`;

  let r;
  try {
    [r] = await Promise.all([api("scan", {}), sleep(calm ? 0 : 700)]);
  } catch (e) {
    busy(false);
    view.innerHTML = `<p class="error">${esc(e.message)}</p><div class="actions"><button class="primary" id="retry">Scan again</button></div>`;
    $("#retry").onclick = showScan;
    return;
  }
  scanResult = r;
  const hw = r.hw;
  const values = {
    cpu: `${hw.cpu_name || hw.arch}${hw.unified ? "" : `, ${hw.cores} cores`}`,
    mem: `${gb(hw.ram_avail)} free of ${gb(hw.ram_total)}`,
    gpu: hw.unified
      ? `Shares system memory, up to ${gb(hw.gpu_usable)} for AI`
      : hw.gpus?.length ? hw.gpus.map((g) => `${g.name}, ${gb(g.mem_free)} free of ${gb(g.mem_total)}`).join("; ") : "No graphics card found, the processor will run it",
    disk: `${gb(hw.disk_free)} free`,
    fit: r.recommended ? `${r.plans.length} way${r.plans.length === 1 ? "" : "s"} to run a model here` : "Nothing fits right now",
  };
  for (const [id] of SCAN_LINES) {
    const li = $("#sc-" + id);
    const ok = !(id === "fit" && !r.recommended);
    li.className = ok ? "ok" : "fail";
    const m = li.querySelector(".mark");
    m.className = "mark";
    m.textContent = ok ? "✓" : "✗";
    li.querySelector(".sub").textContent = values[id];
    await sleep(calm ? 0 : 160);
  }
  busy(false);
  renderPick(r);
}

function machineWord(hw) {
  return hw.os === "darwin" ? "Mac" : hw.os === "windows" ? "PC" : "computer";
}

function speedWord(p) { return { fast: "Fast", good: "Comfortable", slow: "Slow" }[p.speed] || ""; }

// Plain-language description of how a plan uses the hardware.
function howWord(p) {
  const m = p.model;
  const kind = m.moe
    ? `Mixture of experts, ${Math.round(m.params_b)}B total, ${Math.round(m.active_params_b)}B used per word`
    : `Dense, all ${Math.round(m.params_b)}B used per word`;
  const where = {
    gpu: scanResult?.hw.unified ? "fits in shared memory" : "all on the graphics card",
    hybrid: `experts in system memory (${p.n_cpu_moe} of ${m.layers} layers), the rest on the graphics card`,
    split: `${p.ngl} of ${m.layers} layers on the graphics card, the rest on the processor`,
    cpu: "runs on the processor",
  }[p.mode];
  return `${kind}; ${where}`;
}

// Why the recommendation wins, in one line.
function whyWord(rec, plans) {
  const faster = plans.find((p) => p.model.id !== rec.model.id && p.tok_per_sec > rec.tok_per_sec * 1.3);
  if (rec.mode === "hybrid") {
    return `Mixture-of-experts models only touch a few billion parameters per word, so most of <strong>${esc(rec.model.name)}</strong> can sit in system memory with little slowdown. That's how a bigger, smarter model fits next to your graphics card.`;
  }
  if (rec.mode === "gpu" && faster) {
    return `<strong>${esc(rec.model.name)}</strong> is the smartest model that fits. ${esc(faster.model.name)} is faster but less capable.`;
  }
  if (rec.mode === "gpu") return `<strong>${esc(rec.model.name)}</strong> fits entirely in ${scanResult.hw.unified ? "memory" : "graphics memory"}, the fastest way to run it.`;
  if (rec.mode === "cpu") return `With no free graphics memory, <strong>${esc(rec.model.name)}</strong> is the smartest model your processor runs at a usable speed.`;
  return "";
}

// One comparison card: smarts, speed, and where the model's memory goes.
function optCard(p, rec, maxScore) {
  const hw = scanResult.hw;
  const smart = p.model.score / maxScore;
  const speed = Math.min(1, Math.log(1 + p.tok_per_sec) / Math.log(151));
  let bars = `
    <span class="k">Smarts</span><div class="meter smarts"><div class="fill" style="width:0" data-to="${(smart * 100).toFixed(1)}%"></div></div><span class="v">${p.model.score === maxScore ? "smartest here" : ""}</span>
    <span class="k">Speed</span><div class="meter speed ${p.speed === "slow" ? "slow" : ""}"><div class="fill" style="width:0" data-to="${(speed * 100).toFixed(1)}%"></div></div><span class="v">~${words(p.tok_per_sec)} words/s</span>`;
  if (hw.unified) {
    bars += `<span class="k">Memory</span>${memBar(hw.ram_total, hw.ram_total - hw.ram_avail, p.gpu_need)}<span class="v">${gb(p.gpu_need)} of ${gb(hw.ram_total)}</span>`;
  } else {
    if (hw.gpus?.length) {
      const total = hw.gpus.reduce((a, g) => a + g.mem_total, 0);
      const free = hw.gpus.reduce((a, g) => a + g.mem_free, 0);
      bars += `<span class="k">Graphics</span>${memBar(total, total - free, p.gpu_need)}<span class="v">${gb(p.gpu_need)} of ${gb(total)}</span>`;
    }
    bars += `<span class="k">System</span>${memBar(hw.ram_total, hw.ram_total - hw.ram_avail, p.ram_need)}<span class="v">${p.ram_need ? `${gb(p.ram_need)} of ${gb(hw.ram_total)}` : "not used"}</span>`;
  }
  const tag = p.model.id === rec?.model.id ? "Recommended" : !p.disk_ok ? "Not enough disk" : p.speed === "slow" ? "Too slow" : "";
  return `
    <button class="opt reveal ${p.disk_ok ? "" : "nodisk"}" role="radio" aria-checked="${p.model.id === chosenID}" data-id="${esc(p.model.id)}" ${p.disk_ok ? "" : "disabled"}>
      <span class="o-name">${esc(p.model.name)}</span><span class="o-tag">${tag}</span>
      <span class="o-how">${esc(howWord(p))}</span>
      <div class="o-bars">${bars}</div>
    </button>`;
}

const ENGINE = { cuda: "NVIDIA CUDA", metal: "Apple Metal", vulkan: "Vulkan", rocm: "AMD ROCm", cpu: "Processor only" };

// One big memory bar: other apps, then (once a model is picked) the model.
function memRow(label, id, total, used) {
  const pct = (v) => ((Math.max(0, v) / total) * 100).toFixed(2) + "%";
  return `
    <div class="meter-row" data-total="${total}" data-used="${used}" id="${id}">
      <div class="label"><span>${label}</span><span class="mtxt"><b>${gb(total - used)} free</b> of ${gb(total)}</span></div>
      <div class="meter big" data-grow>
        <div class="seg others" style="width:0" data-w="${pct(used)}"></div>
        <div class="seg model" style="left:${pct(used)};width:0"></div>
      </div>
      <div class="mleg"><span><i class="k-others"></i>Other apps ${gb(used)}</span><span class="mleg-model" hidden><i class="k-model"></i><span></span></span><span class="mleg-free"><i class="k-free"></i>Free ${gb(total - used)}</span></div>
    </div>`;
}

// Specs plus the machine's memory pools.
function sysPanel(hw) {
  const g = hw.gpus || [];
  const gpuTotal = g.reduce((a, x) => a + x.mem_total, 0), gpuFree = g.reduce((a, x) => a + x.mem_free, 0);
  const graphics = hw.unified ? "Built into the chip, shares system memory"
    : g.length ? g.map((x) => `${esc(x.name)}${x.driver ? `, driver ${esc(x.driver)}` : ""}`).join("<br>") : "None found";
  const mem = hw.unified
    ? memRow("Memory, shared with graphics", "m-gpu", hw.ram_total, hw.ram_total - hw.ram_avail) +
      `<p class="note" style="margin-top:-6px">macOS lets AI use up to ${gb(hw.gpu_usable)} of it.</p>`
    : (g.length ? memRow("Graphics memory", "m-gpu", gpuTotal, gpuTotal - gpuFree) : "") +
      memRow("System memory", "m-ram", hw.ram_total, hw.ram_total - hw.ram_avail);
  return `
    <section class="sys reveal">
      <dl class="facts">
        <dt>System</dt><dd>${esc(hw.os_name || hw.os)}</dd>
        <dt>Processor</dt><dd>${esc(hw.cpu_name || hw.arch)}, ${hw.cores} threads</dd>
        <dt>Graphics</dt><dd>${graphics}</dd>
        <dt>Engine</dt><dd>${ENGINE[hw.backend] || hw.backend}</dd>
        <dt>Storage</dt><dd>${gb(hw.disk_free)} free${hw.models_dir ? ` in ${esc(hw.models_dir)}` : ""}</dd>
      </dl>
      ${mem}
    </section>`;
}

// Animate the chosen plan's footprint into the system memory bars.
function showFootprint(p) {
  const hw = scanResult.hw;
  const put = (id, need) => {
    const row = $("#" + id);
    if (!row) return;
    const total = +row.dataset.total, used = +row.dataset.used;
    const seg = row.querySelector(".seg.model"), leg = row.querySelector(".mleg-model");
    seg.style.width = ((Math.min(need, total - used) / total) * 100).toFixed(2) + "%";
    leg.hidden = !need;
    leg.querySelector("span").textContent = `This model ${gb(need)}`;
    row.querySelector(".mleg-free span, .mleg-free").lastChild.textContent = `Free after ${gb(Math.max(0, total - used - need))}`;
    row.querySelector(".mtxt").innerHTML = need ? `<b>${gb(need)}</b> for the model, ${gb(Math.max(0, total - used - need))} left` : `<b>${gb(total - used)} free</b> of ${gb(total)}, not used`;
  };
  put("m-gpu", hw.unified ? p.gpu_need : p.mode === "cpu" ? 0 : p.gpu_need);
  if (!hw.unified) put("m-ram", p.ram_need);
}

async function renderPick(r) {
  const hw = r.hw, rec = r.recommended;
  await sleep(calm ? 0 : 250);
  $("#scanlog")?.remove();
  view.insertAdjacentHTML("afterbegin", sysPanel(hw));
  chosenID = rec?.model.id || null;
  const plans = r.plans || [];
  const maxScore = Math.max(...plans.map((p) => p.model.score), 1);

  let head;
  if (rec) {
    head = `
      <div class="pick reveal">
        <p class="lead">Your ${machineWord(hw)} can run</p>
        <p class="name" id="pick-name"></p>
        <dl class="facts" id="pick-facts">${pickFacts(rec)}</dl>
      </div>
      <p class="why-line reveal">${whyWord(rec, plans)}</p>`;
  } else {
    head = `<p class="error reveal">${plans.length
      ? `Not enough free disk space. The smallest model that fits needs ${gb(plans[plans.length - 1].quant.bytes + 2 * GB)} free.`
      : "No model fits in the memory that's free right now. Quit some apps below, then scan again."}</p>`;
  }

  const unlock = r.unlock ? `
    <p class="note reveal">${r.unlock_gpu ? "Your graphics card is busy. " : ""}Quit apps to free about <strong>${Math.ceil(r.unlock_free_gb)} GB</strong> of ${r.unlock_gpu ? "graphics memory" : "memory"} and you could run <strong>${esc(r.unlock.model.name)}</strong> at ~${words(r.unlock.tok_per_sec)} words/s instead.
    <button class="link" id="rescan">Scan again</button></p>` : "";

  const notes = (hw.notes || []).map((n) => `<p class="note reveal">${esc(n)}</p>`).join("");

  view.insertAdjacentHTML("beforeend", `
    ${notes}
    ${head}
    ${unlock}
    <div class="actions reveal">
      ${rec ? `<button class="primary" id="deploy">Set it up<span class="key">enter</span></button>` : `<button class="primary" id="rescan2">Scan again</button>`}
    </div>
    ${plans.length > 1 ? `<h2>Compare the ways to run a model here</h2><div class="opts" role="radiogroup">${plans.slice(0, 6).map((p) => optCard(p, rec, maxScore)).join("")}</div>
      <div class="legend"><span><i class="k-others"></i>Used by other apps</span><span><i class="k-model"></i>The model</span><span><i class="k-free"></i>Free</span></div>` : ""}
    ${procTable(hw)}`);

  growBars(view);
  if (rec) {
    type($("#pick-name"), rec.model.name, 34);
    setTimeout(() => showFootprint(rec), calm ? 0 : 500);
  }
  $("#deploy")?.addEventListener("click", deploy);
  $("#rescan")?.addEventListener("click", showScan);
  $("#rescan2")?.addEventListener("click", showScan);
  $$(".opt").forEach((card) => card.addEventListener("click", () => {
    chosenID = card.dataset.id;
    $$(".opt").forEach((c) => c.setAttribute("aria-checked", c === card ? "true" : "false"));
    const p = plans.find((x) => x.model.id === chosenID);
    type($("#pick-name"), p.model.name, 18);
    $("#pick-facts").innerHTML = pickFacts(p);
    showFootprint(p);
  }));
  $$("[data-kill]").forEach((b) => b.addEventListener("click", async () => {
    if (!confirm(`Quit ${b.dataset.name}? Unsaved work in it may be lost.`)) return;
    b.disabled = true;
    try { await api("kill", { pid: Number(b.dataset.kill) }); } catch (e) { alert(e.message); }
    showScan();
  }));
}

function pickFacts(p) {
  return `
    <dt>Download</dt><dd>${gb(p.quant.bytes)}</dd>
    <dt>Speed</dt><dd>${speedWord(p)}, about ${words(p.tok_per_sec)} words per second</dd>
    <dt>Context</dt><dd>${ctxLabel(p.ctx)} tokens (how much it can read at once)</dd>
    <dt>Tools</dt><dd>${p.model.tool_calling ? "Works with coding agents" : "Chat only"}</dd>`;
}

function procTable(hw) {
  const list = hw.gpu_procs?.length ? hw.gpu_procs : hw.top_mem || [];
  if (!list.length) return "";
  const title = hw.gpu_procs?.length ? "Already using your graphics card" : "Using the most memory right now";
  const max = Math.max(...list.map((p) => p.mem));
  return `
    <h2>${title}</h2>
    <table class="procs">
      ${list.map((p) => `
        <tr>
          <td class="name">${esc(p.name)}<div class="meter thin" style="margin-top:4px;max-width:240px"><div class="fill" style="width:0;background:repeating-linear-gradient(90deg,var(--dim) 0 6px,transparent 6px 8px)" data-to="${((p.mem / max) * 100).toFixed(1)}%"></div></div></td>
          <td class="mem">${gb(p.mem)}</td>
          <td class="act">${p.killable ? `<button class="small danger" data-kill="${p.pid}" data-name="${esc(p.name)}">Quit</button>` : ""}</td>
        </tr>`).join("")}
    </table>`;
}

// ---------- deploy ----------

// Share of the whole setup each step represents, for the overall bar.
const STEPS = [
  ["engine", "Get the AI engine", 0.08],
  ["model", "Download the model", 0.62],
  ["start", "Start your server", 0.04],
  ["load", "Load the model into memory", 0.16],
  ["test", "Ask it a question", 0.10],
];

async function deploy() {
  try {
    await api("deploy", { model_id: chosenID || "" });
  } catch (e) {
    return alert(e.message);
  }
  showDeploy(false);
}

function showDeploy(resumed) {
  busy(true);
  const plan = scanResult?.plans.find((p) => p.model.id === chosenID);
  view.innerHTML = `
    ${resumed ? `<p class="note">Picking up where setup left off.</p>` : ""}
    <div class="overall">
      <div class="head"><span class="title">Setting up ${esc(plan?.model.name || "your model")}</span><span class="pct" id="pct">0%</span></div>
      ${meter("big live", "overall")}
      <div class="foot"><span id="ostat">Starting</span><span id="oeta"></span></div>
    </div>
    <ul class="log" id="steps">
      ${STEPS.map(([id, label]) => `
        <li id="s-${id}"><span class="mark">·</span><span class="text"><span class="lbl">${label}</span><span class="sub"></span><div class="step-meter" hidden>${meter("thin live")}</div><span class="stat"></span></span></li>`).join("")}
    </ul>
    <div id="after"></div>`;

  const frac = Object.fromEntries(STEPS.map(([id]) => [id, 0]));
  const updateOverall = () => {
    const f = STEPS.reduce((a, [id, , w]) => a + w * frac[id], 0);
    setMeter($("#overall"), f);
    $("#pct").textContent = Math.floor(f * 100) + "%";
  };
  const set = (id, state, sub) => {
    const li = $("#s-" + id);
    if (!li) return;
    li.className = state;
    const mark = li.querySelector(".mark");
    mark.className = "mark" + (state === "run" ? " spin" : "");
    mark.textContent = state === "ok" ? "✓" : state === "fail" ? "✗" : state === "run" ? "" : "·";
    if (sub !== undefined) li.querySelector(".sub").textContent = sub;
    if (state === "ok" || state === "fail") {
      li.querySelector(".step-meter").hidden = true;
      li.querySelector(".stat").textContent = "";
      if (state === "ok") frac[id] = 1;
      updateOverall();
    }
  };
  const showStepMeter = (id, f) => {
    const sm = $("#s-" + id + " .step-meter");
    sm.hidden = false;
    const m = sm.querySelector(".meter");
    f == null ? indet(m) : setMeter(m, f);
  };

  let reply = "";
  handlers = (e) => {
    if (e.job !== "deploy") return;
    if (e.step === "done") {
      if (e.state === "ok") {
        frac.test = 1; updateOverall();
        $("#overall").classList.add("ok");
        return setTimeout(() => showStatus(e.data, reply, true), calm ? 0 : 700);
      }
      busy(false);
      $("#overall").classList.add("fail");
      $("#ostat").textContent = "Stopped";
      $("#after").innerHTML = `
        <p class="error">${esc(e.msg)}</p>
        <div class="actions"><button class="primary" id="back">Back to the start</button></div>`;
      $("#back").onclick = showScan;
      return;
    }
    const label = STEPS.find(([id]) => id === e.step)?.[1];
    if (e.state === "progress") {
      if (e.step === "load") {
        // Loading reports no bytes: ease toward 90% so the bar keeps moving.
        const secs = parseInt(e.msg) || 0;
        frac.load = Math.min(0.9, 1 - Math.exp(-secs / 25));
        showStepMeter("load", null);
        $("#s-load .stat").textContent = `${secs}s`;
        $("#ostat").textContent = label;
        updateOverall();
        return;
      }
      if (e.total) {
        frac[e.step] = e.done / e.total;
        showStepMeter(e.step, frac[e.step]);
        const speed = e.bps ? `${(e.bps / 1e6).toFixed(e.bps < 1e7 ? 1 : 0)} MB/s` : "";
        $("#s-" + e.step + " .stat").textContent = `${gb(e.done)} of ${gb(e.total)}   ${speed}   ${eta(e.done, e.total, e.bps)}`;
        $("#ostat").textContent = label;
        if (e.step === "model") $("#oeta").textContent = eta(e.done, e.total, e.bps);
        updateOverall();
      }
      return;
    }
    if (e.state === "run") {
      set(e.step, "run", e.msg);
      $("#ostat").textContent = label || e.msg;
      if (e.step === "start" || e.step === "test") showStepMeter(e.step, null);
    }
    if (e.state === "ok") {
      if (e.step === "test") reply = e.msg;
      set(e.step, "ok", e.step === "test" ? "Answered" : e.msg);
      if (e.step === "model") $("#oeta").textContent = "";
    }
    if (e.state === "fail") set(e.step, "fail", e.msg);
  };
}

// ---------- done / status ----------

async function showStatus(s, reply, fresh) {
  if (!s) s = await api("status");
  handlers = null;
  busy(false);
  if (fresh) celebrate();
  const c = s.config;
  const up = s.healthy;
  const masked = c.key.slice(0, 9) + "•".repeat(12);
  const curl = `curl ${s.local_url}/chat/completions \\
  -H "Authorization: Bearer ${c.key}" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "${c.model_id}", "messages": [{"role": "user", "content": "Hello!"}]}'`;

  view.innerHTML = `
    <div class="live-banner reveal ${up ? "" : "down"}"><span class="pulse"></span><span>${up ? "Your AI is live" : "Your AI is stopped"}</span></div>
    ${reply ? `<p class="note reveal">It says:</p><p class="reply reveal" id="reply"></p>` : ""}

    <div class="endpoint reveal">
      <div class="row" id="row-url"><span class="k">URL</span><code id="url">${fresh ? "" : esc(s.local_url)}</code><button class="small" data-copy="${esc(s.local_url)}">Copy</button></div>
      ${s.lan_url ? `<div class="row"><span class="k">LAN</span><code>${esc(s.lan_url)}</code><button class="small" data-copy="${esc(s.lan_url)}">Copy</button></div>` : ""}
    </div>
    <h2>API keys</h2>
    <table class="keys">
      ${(c.keys || [{ id: "k1", name: "Default", key: c.key }]).map((k, i) => `
        <tr data-id="${esc(k.id)}" ${i === 0 ? 'id="row-key"' : ""}>
          <td class="kname">${esc(k.name)}${i === 0 ? `<span class="kmeta">used by connected apps</span>` : ""}</td>
          <td><code ${i === 0 ? 'id="key"' : ""}>${fresh && i === 0 ? "" : esc(k.key.slice(0, 9) + "•".repeat(12))}</code></td>
          <td class="act">
            <button class="small" data-copy="${esc(k.key)}">Copy</button>
            <button class="small" data-rotate="${esc(k.id)}">Rotate</button>
            ${(c.keys || []).length > 1 ? `<button class="small danger" data-revoke="${esc(k.id)}" data-name="${esc(k.name)}">Revoke</button>` : ""}
          </td>
        </tr>`).join("")}
    </table>
    <form class="newkey" id="newkey">
      <input id="newkey-name" maxlength="40" placeholder="Name it, e.g. Phone or Work laptop" aria-label="New key name">
      <button type="submit">New key</button>
    </form>
    <div id="key-work" hidden>${meter("thin")}<p class="note" id="key-note"></p></div>
    <dl class="facts reveal">
      <dt>Model</dt><dd>${esc(c.model_name)}</dd>
      <dt>Context</dt><dd>${ctxLabel(c.ctx)} tokens</dd>
    </dl>
    <label class="check"><input type="checkbox" id="lan" ${c.lan ? "checked" : ""}> Let other devices on my network use it <span class="why">(phones, other computers)</span></label>

    <h2>Use it in an app</h2>
    <div id="apps-area"></div>

    <details>
      <summary>Use it from code</summary>
      <pre class="snippet">${esc(curl)}</pre>
      <p class="note">Works with any app that supports the OpenAI API: set the base URL and key above.</p>
    </details>

    <div class="actions">
      ${up ? `<button id="stop">Stop server</button>` : `<button id="start">Start server</button>`}
      <button id="change">Change model</button>
      <button class="danger" id="uninstall">Uninstall</button>
    </div>
    <details class="about">
      <summary>About</summary>
      <p class="note">${esc(ATTRIBUTION)}</p>
    </details>`;

  if (fresh) {
    (async () => {
      $("#row-url").classList.add("flash");
      await type($("#url"), s.local_url, 16);
      $("#row-key").classList.add("flash");
      await type($("#key"), masked, 16);
      if (reply) await type($("#reply"), reply, 12);
    })();
  } else if (reply) {
    $("#reply").textContent = reply;
  }

  $$("[data-copy]").forEach((b) => b.addEventListener("click", async () => {
    try { await navigator.clipboard.writeText(b.dataset.copy); b.textContent = "Copied"; }
    catch { b.textContent = "Select and copy"; }
    setTimeout(() => (b.textContent = "Copy"), 1500);
  }));

  renderApps(s);

  // Key changes restart the server, so show progress and re-render after.
  const keyJob = async (path, body, flashID) => {
    $$("#newkey button, [data-rotate], [data-revoke]").forEach((b) => (b.disabled = true));
    $("#key-work").hidden = false;
    indet($("#key-work .meter"));
    busy(true);
    $("#key-note").textContent = "Working…";
    handlers = (ev) => {
      if (ev.job !== "keys") return;
      if (ev.step === "done") {
        if (ev.state === "ok") {
          showStatus(ev.data).then(() => {
            const row = flashID ? $(`tr[data-id="${flashID}"]`) : $(".keys tr:last-child");
            row?.classList.add("flash");
          });
        } else {
          busy(false);
          $("#key-note").textContent = ev.msg;
        }
        return;
      }
      $("#key-note").textContent = ev.msg;
    };
    try { await api(path, body); } catch (err) { busy(false); $("#key-note").textContent = err.message; }
  };
  $("#newkey").onsubmit = (e) => {
    e.preventDefault();
    keyJob("keys/add", { name: $("#newkey-name").value.trim() });
  };
  $$("[data-rotate]").forEach((b) => b.addEventListener("click", () => {
    if (!confirm("Rotate this key? Anything still using the old value will stop working. Apps EZLM connected are updated for you.")) return;
    keyJob("keys/rotate", { id: b.dataset.rotate }, b.dataset.rotate);
  }));
  $$("[data-revoke]").forEach((b) => b.addEventListener("click", () => {
    if (!confirm(`Revoke "${b.dataset.name}"? Anything using it will stop working.`)) return;
    keyJob("keys/revoke", { id: b.dataset.revoke });
  }));

  $("#lan").onchange = async (e) => {
    e.target.disabled = true;
    busy(true);
    $("#int-note").textContent = "Restarting the server…";
    handlers = (ev) => {
      if (ev.job === "lan" && ev.step === "done") {
        if (ev.state === "ok") showStatus(ev.data);
        else { busy(false); $("#int-note").textContent = ev.msg; }
      }
    };
    try { await api("lan", { on: e.target.checked }); } catch (err) { alert(err.message); showStatus(); }
  };
  $("#stop")?.addEventListener("click", async () => { await api("stop", {}).catch(() => {}); setTimeout(() => showStatus(), 800); });
  $("#start")?.addEventListener("click", async () => {
    await api("start", {}).catch((e) => alert(e.message));
    busy(true);
    $("#int-note").textContent = "Starting. Loading the model can take a minute.";
    pollUntilUp();
  });
  $("#change").onclick = showScan;
  $("#uninstall").onclick = async () => {
    if (!confirm("Remove EZLM's server, downloaded models and settings from this computer?")) return;
    await api("uninstall", {}).catch((e) => alert(e.message));
    view.innerHTML = `<p class="note">EZLM's server, models and settings are gone. You can close this page and delete the EZLM app.</p>`;
  };
}

async function pollUntilUp() {
  for (let i = 0; i < 120; i++) {
    await sleep(2000);
    const s = await api("status").catch(() => null);
    if (s?.healthy) return showStatus(s, "", true);
  }
}

// ---------- apps: tiles, Connect, Open ----------

// What the apps area shows besides the engine's status: the newest launch, and
// the reasons tiles failed to connect (kept across re-renders).
const appUI = { status: null, launching: false, id: "", note: "", failed: false, fail: {} };

function appTile(t, s, trade) {
  const cur = appCurrent(t);
  const st = appUI.fail[t.id] ? { text: appUI.fail[t.id], tone: "err" } : appStateLine(t, s);
  const tr = t.id === "hermes" ? trade : null;
  const blocked = !!tr?.blocked || cur === "needs_tools";
  const selectable = t.supported && !blocked;
  const needs64 = t.supported && cur === "needs_64k";
  const broken = t.supported && cur === "broken" && !appUI.fail[t.id];
  const ver = appVersionLine(t), checked = appUI.fail[t.id] ? "" : appCheckedLine(t);
  const id = esc(t.id);
  return `
    <div class="app${t.supported ? "" : " off"}${t.supported && blocked ? " blocked" : ""}" data-id="${id}" role="button" tabindex="${selectable ? 0 : -1}" aria-pressed="false" aria-disabled="${!selectable}">
      <span class="app-head">${appLogo(t, 36, true)}<span class="app-name">${esc(t.name)}</span></span>
      ${ver ? `<span class="app-ver">${esc(ver)}</span>` : ""}
      <span class="app-desc">${esc(t.desc)}</span>
      <span class="app-state ${st.tone}">${esc(st.text)}</span>
      ${checked ? `<span class="app-check">${esc(checked)}</span>` : ""}
      ${tr && (cur !== "needs_64k" || tr.warn) ? `<span class="app-trade ${tr.warn ? "warn" : ""}">${esc(tr.text)}</span>` : ""}
      ${needs64 || broken ? `<span class="app-acts">${needs64 ? fix64(t, tr) : ""}${broken ? `<button class="small" data-log="${id}">Show details</button><button class="small" data-report="${id}">Report a problem</button>` : ""}</span>` : ""}
      ${meter("thin")}
    </div>`;
}

// What the 64K sentence says: connect again, or a smaller model when this one
// can't have 64K.
function fix64(t, trade, cls = "small") {
  return trade?.blocked
    ? `<button class="${cls}" data-pick="1">Pick a smaller model</button>`
    : `<button class="${cls}" data-give64="${esc(t.id)}">Connect again</button>`;
}

function renderApps(s) {
  const area = $("#apps-area");
  if (!area || !s) return;
  appUI.status = s;
  const mkdefault = $("#mkdefault")?.checked ?? false;
  const on = s.targets.filter((t) => appCurrent(t) === "connected");
  const trade = hermesTrade(s);
  area.innerHTML = `
    <p class="note connected">${on.length ? `Connected to ${on.map((t) => `<span class="capp">${appLogo(t, 20)}${esc(t.name)}</span>`).join("")}` : "Not connected to any apps yet."}</p>
    <div class="apps">${s.targets.map((t) => appTile(t, s, trade)).join("")}</div>
    <label class="check"><input type="checkbox" id="mkdefault" ${mkdefault ? "checked" : ""}> Make this the default model in those apps</label>
    <div class="actions">
      <button class="primary" id="connect" disabled>Connect<span class="key">enter</span></button>
      <a class="btn" href="http://127.0.0.1:${s.config.port}/" target="_blank" rel="noopener">Open chat</a>
    </div>
    <p class="note" id="int-note"></p>
    <div id="launch-area"></div>
    <section class="panel" id="applog" hidden aria-live="polite"></section>
    <form class="panel" id="report" hidden></form>`;

  const selected = () => $$(".app[aria-pressed='true']").map((x) => x.dataset.id);
  const toggle = (tile) => {
    if (tile.classList.contains("working") || tile.getAttribute("aria-disabled") === "true" || $(".app.working")) return;
    tile.setAttribute("aria-pressed", tile.getAttribute("aria-pressed") === "true" ? "false" : "true");
    $("#connect").disabled = !selected().length;
  };
  $$(".app", area).forEach((tile) => {
    tile.addEventListener("click", (e) => { if (!e.target.closest("button")) toggle(tile); });
    tile.addEventListener("keydown", (e) => {
      if ((e.key !== "Enter" && e.key !== " ") || e.target !== tile) return;
      e.preventDefault(); e.stopPropagation();
      toggle(tile);
    });
  });
  $("#connect").onclick = () => connect(selected(), $("#mkdefault").checked);
  if (!area.dataset.bound) area.addEventListener("click", (e) => {
    const b = e.target.closest("button");
    if (!b) return;
    if (b.dataset.give64) connect([b.dataset.give64], $("#mkdefault").checked);
    if (b.dataset.pick) showScan();
    if (b.dataset.log) showAppLog(b.dataset.log);
    if (b.dataset.report) reportApp(b.dataset.report, appStateLine(appUI.status.targets.find((t) => t.id === b.dataset.report) || {}, appUI.status).text);
    if (b.dataset.launch !== undefined) launchApp(b.dataset.launch, b.dataset.mode);
  });
  area.dataset.bound = "1";
  paintLaunch();
}

// "Use it": one button per way to open each app that runs here, what the launch
// is doing, and when it fails, why and what to do next.
function paintLaunch() {
  const s = appUI.status, area = $("#launch-area");
  if (!area || !s) return;
  const targets = s.targets.filter((t) => t.supported);
  const t = targets.find((x) => x.id === appUI.id);
  const trade = hermesTrade(s);
  area.innerHTML = `
    <div class="launch">${targets.map((t) => appModes(t).map((m, i) => `
      <button class="${i ? "small" : ""}" data-launch="${esc(t.id)}" data-mode="${i ? esc(m.id) : ""}" ${appUI.launching || !s.healthy ? "disabled" : ""}>${appLogo(t, 16)}<span>${esc(m.label)}</span></button>`).join("")).join("")}
    </div>
    ${appUI.note ? `<p class="launch-note ${appUI.failed ? "err" : ""}">${appUI.launching ? `<span class="spin" aria-hidden="true"></span>` : ""}${esc(appUI.note)}</p>` : ""}
    ${appUI.failed && !appUI.launching && t ? `<div class="launch-acts">
      ${appCurrent(t) === "needs_64k" ? fix64(t, t.id === "hermes" ? trade : null, "") : ""}
      <button class="small" data-log="${esc(t.id)}">Show details</button>
      <button class="small" data-report="${esc(t.id)}" data-reason="1">Report a problem</button>
    </div>` : ""}`;
  // The launch's own reason is better than the tile's for a report from here.
  const r = $("[data-reason]", area);
  if (r) r.onclick = (e) => { e.stopPropagation(); reportApp(t.id, appUI.note); };
}

async function launchApp(id, mode) {
  const t = appUI.status?.targets.find((x) => x.id === id);
  if (!t || appUI.launching) return;
  Object.assign(appUI, { launching: true, id, failed: false, note: `Opening ${t.name}…` });
  paintLaunch();
  busy(true);
  handlers = (e) => {
    if (e.job !== "launch") return;
    if (e.step === "done") {
      appUI.launching = false;
      busy(false);
      if (e.state !== "ok") {
        appUI.failed = e.state !== "cancelled";
        const why = e.msg || "";
        // The engine's reasons can start lowercase (they're Go errors): this one stands alone.
        appUI.note = e.state === "cancelled" ? "Cancelled." : why ? why[0].toUpperCase() + why.slice(1) : "Couldn't open it.";
      }
      api("status").then(renderApps).catch(paintLaunch); // the app may be installed and connected now
      return;
    }
    if (e.msg) { appUI.note = e.msg; paintLaunch(); }
  };
  try { await api("launch", mode ? { id, mode } : { id }); }
  catch (err) {
    Object.assign(appUI, { launching: false, failed: true, note: err.message });
    busy(false);
    paintLaunch();
  }
}

const appLines = (id) => api("integrate/log?id=" + encodeURIComponent(id)).then((r) => r.lines || []).catch(() => []);
const appName = (id) => appUI.status?.targets.find((t) => t.id === id)?.name || id;
// Keys never belong in a report (the engine's log has none; this is a second net).
const hideKeys = (s) => s.replace(/\b(ezlm-|sk-)[A-Za-z0-9_-]{6,}/g, (k) => k.slice(0, 9) + "…").replace(/Bearer\s+\S+/g, "Bearer …");

// "Show details": the app's connection log, newest last.
async function showAppLog(id) {
  const lines = await appLines(id), name = appName(id), p = $("#applog");
  $("#report").hidden = true;
  p.innerHTML = `
    <h3>${esc(name)}: connection log</h3>
    <p class="note">What EZLM did and what ${esc(name)} said, newest last. Keys are never written here.</p>
    <pre class="log-lines">${esc(lines.length ? lines.join("\n") : "Nothing logged yet.")}</pre>
    <div class="actions">
      <button id="log-close">Close</button>
      <button id="log-copy">Copy</button>
      <button id="log-report">Report a problem</button>
    </div>`;
  p.hidden = false;
  const pre = $(".log-lines", p);
  pre.scrollTop = pre.scrollHeight;
  $("#log-close").onclick = () => { p.hidden = true; };
  $("#log-copy").onclick = async (e) => {
    try { await navigator.clipboard.writeText(lines.join("\n")); e.target.textContent = "Copied"; } catch { e.target.textContent = "Select and copy"; }
    setTimeout(() => (e.target.textContent = "Copy"), 1500);
  };
  $("#log-report").onclick = () => {
    const t = appUI.status?.targets.find((x) => x.id === id);
    reportApp(id, appUI.failed && appUI.id === id ? appUI.note : t ? appStateLine(t, appUI.status).text : `a problem with ${name}`);
  };
  p.scrollIntoView({ block: "nearest", behavior: calm ? "auto" : "smooth" });
}

// "Report a problem" for an app: the reason first, then the end of its log.
async function reportApp(id, reason) {
  const name = appName(id), tail = (await appLines(id)).slice(-40), f = $("#report");
  let text = `${name}: ${reason}`;
  if (tail.length) text += `\n\n${name} log (last ${tail.length} lines):\n` + tail.join("\n");
  $("#applog").hidden = true;
  f.innerHTML = `
    <h3>Report a problem</h3>
    <p class="note">This posts on EZLM's public GitHub page, where the people testing EZLM read it. Thanks for helping.</p>
    <textarea id="report-text" rows="9" aria-label="What happened"></textarea>
    <label class="check"><input type="checkbox" id="report-details" checked> Include technical details: your hardware, versions and recent logs. Keys and your files are never included.</label>
    <div class="actions">
      <button type="submit" class="primary">Send report</button>
      <button type="button" id="report-cancel">Cancel</button>
    </div>
    <p class="note" id="report-note"></p>`;
  $("#report-text").value = hideKeys(text);
  f.hidden = false;
  $("#report-cancel").onclick = () => { f.hidden = true; };
  f.onsubmit = async (e) => {
    e.preventDefault();
    const body = $("#report-text").value.trim(), note = $("#report-note");
    if (!body) { note.textContent = "Say what happened first, even in a few words."; return; }
    note.textContent = "Sending your report…";
    try {
      const r = await api("report", { text: body, details: $("#report-details").checked, app_log: "" });
      if (r.sent) { f.hidden = true; $("#int-note").textContent = `Sent. Thank you! (report #${r.number})`; return; }
      window.open(r.url, "_blank", "noopener");
      note.textContent = "Your report is filled in on GitHub: press Submit there to send it.";
    } catch (err) { note.textContent = "Couldn't send the report: " + err.message; }
  };
  f.scrollIntoView({ block: "nearest", behavior: calm ? "auto" : "smooth" });
  $("#report-text").focus({ preventScroll: true });
}

async function connect(ids, makeDefault) {
  if (!ids.length) return;
  $("#connect").disabled = true;
  busy(true);
  for (const id of ids) {
    delete appUI.fail[id];
    const tile = $(`.app[data-id="${id}"]`);
    if (!tile) continue;
    tile.classList.remove("finished");
    tile.classList.add("working");
    tile.setAttribute("aria-pressed", "true");
    $(".app-acts", tile)?.remove();
    $(".app-check", tile)?.remove();
    const st = tile.querySelector(".app-state");
    st.className = "app-state";
    st.textContent = "Waiting…";
    tile.querySelector(".meter").className = "meter thin";
    setMeter(tile.querySelector(".meter"), 0);
  }
  handlers = (e) => {
    if (e.job !== "integrate") return;
    if (e.step === "done") {
      busy(false);
      const done = e.state === "ok" ? "Done. Open the app and pick the model named EZLM." : e.state === "cancelled" ? "Cancelled." : "";
      if (e.state === "ok") celebrate();
      // Show what the engine now says about each app (states, versions, checks).
      api("status").then(renderApps).catch(() => {}).finally(() => { $("#int-note").textContent = done; });
      return;
    }
    const tile = $(`.app[data-id="${e.step.replace("integrate:", "")}"]`);
    if (!tile) return;
    const st = tile.querySelector(".app-state"), m = tile.querySelector(".meter");
    if (e.state === "run" || e.state === "info") {
      st.textContent = e.msg.replace(/…$/, "") + "…";
      // Installing and testing take unknown time; writing the config is quick.
      if (/install|test/i.test(e.msg)) indet(m);
      else if (/provider/i.test(e.msg)) { setMeter(m, 0.55); m.classList.add("live"); }
    }
    if (e.state === "ok") {
      tile.classList.replace("working", "finished");
      m.classList.remove("indet", "live"); m.classList.add("ok"); setMeter(m, 1);
      st.className = "app-state on"; st.textContent = "✓ Connected and answering";
      tile.setAttribute("aria-pressed", "false");
    }
    if (e.state === "fail") {
      appUI.fail[e.step.replace("integrate:", "")] = e.msg;
      tile.classList.replace("working", "finished");
      m.classList.remove("indet", "live"); m.classList.add("fail"); setMeter(m, 1);
      st.className = "app-state err"; st.textContent = e.msg;
    }
  };
  try { await api("integrate", { ids, default: makeDefault }); }
  catch (e) {
    busy(false);
    $$(".app.working").forEach((t) => t.classList.remove("working"));
    $("#int-note").textContent = e.message;
    $("#connect").disabled = false;
  }
}

$("#foot").textContent = "Everything runs on this computer. Nothing you type is sent anywhere else.";
boot();
