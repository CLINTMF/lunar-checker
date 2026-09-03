const $ = (id) => document.getElementById(id);
const form = $("form");
const button = $("button");
let currentServer = "donut";

const labels = {
  donut: [
    ["money", "Money", "◆"], ["shards", "Shards", "◇"], ["playtime", "Playtime", "◷"],
    ["kills", "Kills", "⚔"], ["deaths", "Deaths", "✕"], ["kd", "K/D", "✦"],
    ["blocksPlaced", "Blocks placed", "▦"], ["blocksBroken", "Blocks broken", "▧"],
    ["mobsKilled", "Mobs killed", "☠"], ["moneySpent", "Money spent", "↘"], ["moneyMade", "Money made", "↗"]
  ],
  hypixel: [
    ["networkLevel", "Network level", "✦"], ["karma", "Karma", "◇"], ["achievementPoints", "Achievements", "★"],
    ["playing", "Current status", "●"]
  ]
};

const apiBase = (new URLSearchParams(location.search).get("api") || location.origin).replace(/\/$/, "");

function setServer(server) {
  currentServer = server;
  document.querySelectorAll(".server").forEach((button) => {
    const active = button.dataset.server === server;
    button.classList.toggle("active", active);
    button.setAttribute("aria-selected", active ? "true" : "false");
  });
  $("status").textContent = server === "hypixel"
    ? "Hypixel SkyBlock stats · official API adapter"
    : "DonutSMP economy and PvP stats · live scraper";
}

document.querySelectorAll(".server").forEach((button) => {
  button.addEventListener("click", () => setServer(button.dataset.server));
});

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const username = $("username").value.trim();
  if (!username) {
    $("status").textContent = "Enter a Minecraft username.";
    $("status").className = "status error";
    return;
  }
  button.disabled = true;
  $("status").textContent = `Fetching ${currentServer === "donut" ? "DonutSMP" : "Hypixel"} stats…`;
  $("status").className = "status";
  try {
    const response = await fetch(`${apiBase}/api/minecraft/${currentServer}/${encodeURIComponent(username)}`);
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "Stats request failed");
    render(body.data, body.cached);
    $("status").textContent = body.cached ? "Showing cached stats · refreshes automatically" : "Live stats loaded";
  } catch (error) {
    $("result").classList.remove("show");
    $("status").textContent = error.message;
    $("status").className = "status error";
  } finally {
    button.disabled = false;
  }
});

function render(data, cached) {
  $("skin").src = data.skinUrl || `https://mc-heads.net/avatar/${encodeURIComponent(data.username)}/128`;
  $("skin").alt = `${data.username} Minecraft skin`;
  $("player-name").textContent = data.username;
  $("profile-status").textContent = `${data.status || "Unknown"} · ${data.server}${cached ? " · cached" : ""}`;
  $("profile-source").textContent = `Live data from ${data.source || data.server}`;
  $("profile-link").href = data.profileUrl || "#";
  $("fetched-at").textContent = data.fetchedAt ? new Date(data.fetchedAt).toLocaleTimeString() : "";
  const fields = labels[currentServer].filter(([key]) => data.stats && data.stats[key] !== undefined);
  $("stats").innerHTML = fields.map(([key, label, icon]) =>
    `<article class="card"><div class="label"><span class="icon">${icon}</span>${label}</div><div class="value">${escapeHTML(String(data.stats[key]))}</div></article>`
  ).join("");
  $("result").classList.add("show");
}

function escapeHTML(value) {
  return value.replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;" }[char]));
}

setServer("donut");