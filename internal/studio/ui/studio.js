"use strict";

const session = location.hash.slice(1);
const form = document.querySelector("#create");
const submit = document.querySelector("#submit");
const folder = document.querySelector("#folder");
const quit = document.querySelector("#quit");
const message = document.querySelector("#message");
const projects = document.querySelector("#projects");

function show(text, error = false) {
  message.textContent = text;
  message.classList.toggle("error", error);
}

async function request(path, body) {
  const headers = { "X-AppRivet-Session": session };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(path, {
    method: path === "/api/state" ? "GET" : "POST",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return;
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || "AppRivet could not complete this action.");
  return result;
}

async function refresh() {
  const state = await request("/api/state");
  document.querySelector("#workspace-path").textContent = state.workspace;
  document.querySelector("#empty").hidden = state.projects.length !== 0;
  projects.replaceChildren(...state.projects.map(name => {
    const row = document.createElement("li");
    row.textContent = name;
    return row;
  }));
}

form.addEventListener("submit", async event => {
  event.preventDefault();
  submit.disabled = true;
  show("Creating your app…");
  try {
    const result = await request("/api/create", { name: form.elements.namedItem("name").value, bundle_id: form.elements.namedItem("bundle").value });
    show(`Created ${result.suggested_scheme}.`);
    await refresh();
  } catch (error) {
    show(error.message, true);
  } finally {
    submit.disabled = false;
  }
});

folder.addEventListener("click", async () => {
  try { await request("/api/open-folder"); }
  catch (error) { show(error.message, true); }
});

quit.addEventListener("click", async () => {
  try {
    await request("/api/quit");
    for (const control of document.querySelectorAll("button, input")) control.disabled = true;
    show("AppRivet has quit. You can close this tab.");
  } catch (error) { show(error.message, true); }
});

if (!session) {
  for (const control of document.querySelectorAll("button, input")) control.disabled = true;
  show("Start AppRivet from your desktop to open a new session.", true);
} else {
  refresh().catch(error => show(error.message, true));
}
