const status = document.getElementById("status");
const detail = document.getElementById("status-detail");

async function checkHealth() {
  try {
    const response = await fetch("/api/v1/health", { cache: "no-store" });
    const health = await response.json();
    if (response.ok && health.state === "ready") {
      status.textContent = "Connected";
      status.dataset.state = "ready";
      detail.textContent = "Retained evidence is available to this local app.";
      return;
    }
  } catch (_) {
    // The browser may have lost its connection to the local server.
  }
  status.textContent = "Unavailable";
  status.dataset.state = "unavailable";
  detail.textContent = "The evidence database is unavailable. Check the server terminal.";
}

checkHealth();
setInterval(checkHealth, 10000);
