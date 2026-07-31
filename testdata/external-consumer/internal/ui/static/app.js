const status = document.querySelector("#status");

fetch("/healthz", { headers: { Accept: "application/json" } })
  .then((response) => {
    if (!response.ok) {
      throw new Error(`health returned ${response.status}`);
    }
    return response.json();
  })
  .then((health) => {
    status.textContent = `${health.application.name} ${health.application.version} is ${health.status}.`;
  })
  .catch(() => {
    status.textContent = "Application health is unavailable.";
  });
