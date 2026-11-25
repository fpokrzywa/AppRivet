"use strict";

const session = location.hash.slice(1);
const form = document.querySelector("#create");
const fields = document.querySelector("#create-fields");
const submit = document.querySelector("#submit");
const folder = document.querySelector("#folder");
const quit = document.querySelector("#quit");
const message = document.querySelector("#message");
const projects = document.querySelector("#projects");
const brief = form.elements.namedItem("brief");
const nameField = form.elements.namedItem("name");
const starter = form.elements.namedItem("starter");

const starters = {
  auto: ["Automatic from brief", "Match a checklist, saved note or counter. Other ideas start with a blank SwiftUI project and the brief."],
  checklist: ["Checklist", "Add items, check them off, and swipe to delete. Items are saved on the device."],
  notes: ["Notes", "Start with a note editor that saves your text on the device."],
  counter: ["Counter", "Add, subtract or reset a saved count."],
  blank: ["Blank", "Start with a SwiftUI screen you can extend in Xcode."],
};
const examples = {
  checklist: ["TripPack", "A packing checklist for weekend trips. I want to add items, check them off, and keep the list when I reopen the app."],
  notes: ["FieldNotes", "A notes app for jotting down observations on walks. Keep my note when I reopen the app."],
  counter: ["RepCounter", "A counter for tracking exercise reps. I need to add one, undo a rep, and reset between sets."],
};

function show(text, error = false) {
  message.textContent = text;
  message.classList.toggle("error", error);
}

function updateBrief() {
  document.querySelector("#brief-count").textContent = `${brief.value.length.toLocaleString()} / 2,000`;
  const [title, description] = starters[starter.value];
  document.querySelector("#starter-title").textContent = title;
  document.querySelector("#starter-copy").textContent = description;
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
  const rows = document.createDocumentFragment();
  for (const name of state.projects) {
    const row = document.createElement("li");
    row.textContent = name;
    rows.append(row);
  }
  projects.replaceChildren(rows);
}

brief.addEventListener("input", updateBrief);
starter.addEventListener("change", updateBrief);
document.querySelector("#go-projects").addEventListener("click", () => {
  const heading = document.querySelector("#projects-heading");
  heading.focus({ preventScroll: true });
  heading.scrollIntoView({ block: "start" });
});
for (const button of document.querySelectorAll("[data-example]")) {
  button.addEventListener("click", () => {
    const [sampleName, sampleBrief] = examples[button.dataset.example];
    const replacingExample = Object.values(examples).some(([exampleName]) => exampleName === nameField.value);
    if (!nameField.value.trim() || replacingExample) nameField.value = sampleName;
    brief.value = sampleBrief;
    starter.value = button.dataset.example;
    updateBrief();
    brief.focus();
  });
}

form.addEventListener("submit", async event => {
  event.preventDefault();
  submit.disabled = true;
  show("Creating the SwiftUI starter…");
  try {
    const result = await request("/api/create", {
      name: nameField.value,
      bundle_id: form.elements.namedItem("bundle").value,
      brief: brief.value,
      starter: starter.value,
    });
    const label = starters[result.starter]?.[0] ?? "SwiftUI";
    show(`Created ${result.suggested_scheme} with the ${label} starter.`);
    await refresh().catch(() => {});
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
    fields.disabled = folder.disabled = quit.disabled = true;
    show("AppRivet has quit. You can close this tab.");
  } catch (error) { show(error.message, true); }
});

if (!session) {
  show("Start AppRivet from your desktop to open a new session.", true);
} else {
  refresh().then(() => {
    fields.disabled = folder.disabled = quit.disabled = false;
    show("");
  }).catch(error => show(error.message, true));
}
