"use strict";

const byID = id => document.getElementById(id);
const session = location.hash.slice(1);
const form = byID("create");
const fields = byID("create-fields");
const submit = byID("submit");
const folder = byID("folder");
const quit = byID("quit");
const message = byID("message");
const projects = byID("projects");
const brief = byID("brief");
const nameField = byID("name");
const starter = byID("starter");
const format = byID("format");
const admin = byID("admin");
const adminButton = byID("open-admin");
const provider = byID("provider");
const model = byID("model");
const customModel = byID("custom-model");
const apiKey = byID("api-key");
const keyStatus = byID("key-status");
const clearKey = byID("clear-key");
const review = byID("prompt-review");
const reviewStatus = byID("review-status");
const promptText = byID("prompt-text");
const confirmCreate = byID("confirm-create");
const editBrief = byID("edit-brief");
const importStatus = byID("import-status");

let reviewedRequest;
let creating = false;
let activeProvider = "openai";
let flutterSDK = false;
let buildSelection;
let building = false;

const platformNames = {ios: "iOS", android: "Android", web: "Web", windows: "Windows", macos: "macOS", linux: "Linux"};

const providers = {
  openai: {name: "OpenAI", models: [["gpt-6.2-sol", "GPT-6.2 Sol"], ["gpt-6.2-astra", "GPT-6.2 Astra"]]},
  anthropic: {name: "Anthropic", models: [["claude-opus-5-5", "Claude Opus 5.5"], ["claude-fable-5-2", "Claude Fable 5.2"]]},
};
// Keys live in page memory. They are never written to browser storage or the URL.
const profiles = {
  openai: {model: providers.openai.models[0][0], key: ""},
  anthropic: {model: providers.anthropic.models[0][0], key: ""},
};
const keyPattern = /^[A-Za-z0-9_-]{8,512}$/;

const starters = {
  ai: ["AI-generated SwiftUI", "A model writes the app’s SwiftUI source from your brief. Review the source before building in Xcode."],
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

function show(element, text, error = false) {
  element.textContent = text;
  element.classList.toggle("error", error);
}

function updateBrief() {
  byID("brief-count").textContent = brief.value.length.toLocaleString() + " characters";
  const isFlutter = format.value === "flutter";
  const [title, description] = isFlutter
    ? starter.value === "ai"
      ? ["AI-generated Flutter", "The selected model writes shared Dart code for the platforms you checked."]
      : ["Flutter starter", "One shared Dart project with build files for the platforms you checked."]
    : starters[starter.value];
  byID("starter-title").textContent = title;
  byID("starter-copy").textContent = description;
  byID("output-title").textContent = isFlutter ? "Flutter project" : "Xcode project";
  byID("output-source").textContent = isFlutter ? "Shared Dart code in lib/main.dart" : "Editable SwiftUI source";
  byID("run-heading").textContent = isFlutter ? "Build your app" : "Run in Simulator";
  byID("run-copy").textContent = isFlutter
    ? "Pick a build target beside a saved project. Targets that require another host use the same project folder."
    : "On your Mac, open the project in AppRivet and choose Build & Run.";
  byID("flutter-sdk-help").hidden = flutterSDK;
  const settings = profiles[activeProvider];
  byID("selected-model").textContent = starter.value === "ai"
    ? providers[activeProvider].name + " · " + settings.model + (settings.key ? "" : " · Add a key in Admin")
    : "";
}

function updateFormat() {
  const isFlutter = format.value === "flutter";
  byID("platform-settings").hidden = !isFlutter;
  byID("bundle-settings").hidden = isFlutter;
  for (const name of ["auto", "checklist", "notes", "counter"]) {
    starter.querySelector('option[value="' + name + '"]').hidden = isFlutter;
  }
  starter.querySelector('option[value="blank"]').textContent = isFlutter ? "Flutter starter" : "Blank";
  if (starter.value !== "ai") starter.value = isFlutter ? "blank" : "auto";
  updateBrief();
}

async function request(path, body) {
  const headers = {"X-AppRivet-Session": session};
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
  flutterSDK = state.flutter_sdk;
  updateBrief();
  byID("workspace-path").textContent = state.workspace;
  byID("empty").hidden = state.projects.length !== 0;
  const rows = document.createDocumentFragment();
  for (const name of state.projects) {
    const row = document.createElement("li");
    row.textContent = name;
    const flutter = state.flutter_projects.find(project => project.name === name);
    if (flutter) {
      const targets = document.createElement("p");
      targets.className = "field-help";
      targets.textContent = flutter.platforms.map(platform => platformNames[platform]).join(", ");
      row.append(targets);
      if (flutterSDK && flutter.build_targets.length) {
        const controls = document.createElement("div");
        controls.className = "project-build";
        const select = document.createElement("select");
        select.setAttribute("aria-label", "Build target for " + name);
        select.append(...flutter.build_targets.map(platform => new Option(platformNames[platform], platform)));
        const build = document.createElement("button");
        build.type = "button";
        build.className = "quiet";
        build.textContent = "Build";
        build.addEventListener("click", () => openBuild(name, select.value));
        controls.append(select, build);
        row.append(controls);
      }
    }
    rows.append(row);
  }
  projects.replaceChildren(rows);
}

const buildDialog = byID("build-review");
const trusted = byID("trust-build");
const confirmBuild = byID("confirm-build");
const closeBuild = byID("close-build");
const buildStatus = byID("build-status");

function openBuild(name, target) {
  buildSelection = {name, target};
  trusted.checked = false;
  confirmedTrust();
  byID("build-summary").textContent = name + " · " + platformNames[target];
  show(buildStatus, "");
  buildDialog.showModal();
}

function confirmedTrust() {
  confirmBuild.disabled = !trusted.checked || building;
}

trusted.addEventListener("change", confirmedTrust);
closeBuild.addEventListener("click", () => buildDialog.close());
buildDialog.addEventListener("cancel", event => { if (building) event.preventDefault(); });
confirmBuild.addEventListener("click", async () => {
  if (!buildSelection || !trusted.checked || building) return;
  building = true;
  trusted.disabled = closeBuild.disabled = true;
  confirmedTrust();
  show(buildStatus, "Building " + platformNames[buildSelection.target] + "… Flutter may download build dependencies.");
  try {
    const result = await request("/api/build", {...buildSelection, trusted: true});
    show(buildStatus, "Build ready\n" + result.artifact);
  } catch (error) {
    show(buildStatus, error.message, true);
  } finally {
    building = false;
    trusted.disabled = closeBuild.disabled = false;
    confirmedTrust();
  }
});

function watchFile(button, input, consume) {
  button.addEventListener("click", () => input.click());
  input.addEventListener("change", () => {
    const file = input.files[0];
    input.value = "";
    if (file) void consume(file);
  });
}

async function readTextFile(file, extension, maxBytes) {
  if (!extension.test(file.name)) throw new Error("Choose a file in a supported format.");
  if (maxBytes !== undefined && file.size > maxBytes) throw new Error("This file is too large.");
  try {
    return new TextDecoder("utf-8", {fatal: true}).decode(await file.arrayBuffer()).replace(/\r\n?/g, "\n");
  } catch {
    throw new Error("Choose a file saved as UTF-8 plain text.");
  }
}

watchFile(byID("import-brief"), byID("brief-file"), async file => {
  try {
    const contents = await readTextFile(file, /\.(txt|md|markdown)$/i);
    if (!contents.trim() || /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/u.test(contents)) {
      throw new Error("Choose a file containing plain text or Markdown.");
    }
    brief.value = contents;
    updateBrief();
    show(importStatus, "Imported " + file.name + ". You can edit it above.");
    brief.focus();
  } catch (error) {
    show(importStatus, error.message, true);
  }
});

function refreshKeyStatus() {
  const hasKey = Boolean(profiles[provider.value].key);
  show(keyStatus, hasKey ? "Key set for this session. Paste or import a replacement." : "No key set for this provider.");
  clearKey.disabled = !hasKey;
}

function updateCustomField() {
  byID("custom-model-field").hidden = model.value !== "custom";
}

function showProviderSettings() {
  const saved = profiles[provider.value];
  const models = providers[provider.value].models;
  model.replaceChildren(...models.map(([id, label]) => new Option(label, id)), new Option("Custom model ID", "custom"));
  model.value = models.some(([id]) => id === saved.model) ? saved.model : "custom";
  customModel.value = model.value === "custom" ? saved.model : "";
  apiKey.value = "";
  updateCustomField();
  refreshKeyStatus();
}

adminButton.addEventListener("click", () => {
  provider.value = activeProvider;
  showProviderSettings();
  admin.showModal();
});
admin.addEventListener("close", () => { apiKey.value = ""; });
byID("close-admin").addEventListener("click", () => admin.close());
provider.addEventListener("change", showProviderSettings);
model.addEventListener("change", updateCustomField);
clearKey.addEventListener("click", () => {
  profiles[provider.value].key = "";
  apiKey.value = "";
  refreshKeyStatus();
  updateBrief();
});
watchFile(byID("import-key"), byID("key-file"), async file => {
  const selected = provider.value;
  try {
    const key = (await readTextFile(file, /\.txt$/i, 1024)).trim();
    if (!keyPattern.test(key)) throw new Error("Choose a text file containing just one API key.");
    if (!admin.open || provider.value !== selected) return;
    apiKey.value = key;
    show(keyStatus, "Key loaded. Save settings to use it.");
  } catch (error) {
    show(keyStatus, error.message, true);
  }
});
byID("admin-form").addEventListener("submit", event => {
  event.preventDefault();
  const modelID = model.value === "custom" ? customModel.value.trim() : model.value;
  const key = apiKey.value.trim();
  if (!/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(modelID)) {
    show(keyStatus, "Enter a valid model ID from the provider.", true);
    customModel.focus();
    return;
  }
  if (key && !keyPattern.test(key)) {
    show(keyStatus, "Enter a valid API key.", true);
    apiKey.focus();
    return;
  }
  activeProvider = provider.value;
  profiles[activeProvider].model = modelID;
  if (key) profiles[activeProvider].key = key;
  admin.close();
  updateBrief();
});

brief.addEventListener("input", updateBrief);
starter.addEventListener("change", updateBrief);
format.addEventListener("change", updateFormat);
byID("check-sdk").addEventListener("click", () => refresh().catch(error => show(message, error.message, true)));
byID("go-projects").addEventListener("click", () => {
  const heading = byID("projects-heading");
  heading.focus({preventScroll: true});
  heading.scrollIntoView({block: "start"});
});
for (const button of document.querySelectorAll("[data-example]")) {
  button.addEventListener("click", () => {
    const [sampleName, sampleBrief] = examples[button.dataset.example];
    const replacingExample = Object.values(examples).some(([exampleName]) => exampleName === nameField.value);
    if (!nameField.value.trim() || replacingExample) nameField.value = sampleName;
    brief.value = sampleBrief;
    if (format.value !== "flutter" && starter.value !== "ai") starter.value = button.dataset.example;
    show(importStatus, "");
    updateBrief();
    brief.focus();
  });
}

form.addEventListener("submit", async event => {
  event.preventDefault();
  const ai = starter.value === "ai";
  const isFlutter = format.value === "flutter";
  const settings = profiles[activeProvider];
  const project = {
    name: nameField.value,
    bundle_id: isFlutter ? "" : byID("bundle").value,
    format: format.value,
    platforms: isFlutter ? Array.from(form.querySelectorAll('input[name="platform"]:checked'), input => input.value) : ["ios"],
    brief: brief.value,
    ...(ai ? {provider: activeProvider, model: settings.model} : {starter: starter.value}),
  };
  fields.disabled = true;
  show(message, "Validating the brief and project settings…");
  try {
    const draft = await request("/api/review", project);
    reviewedRequest = ai ? {...project, approval: draft.approval} : project;
    promptText.value = draft.prompt;
    const platforms = isFlutter ? " Flutter targets: " + draft.platforms.map(platform => platformNames[platform]).join(", ") + "." : "";
    byID("review-target").textContent = (ai
      ? "Destination: " + providers[draft.provider].name + " · " + draft.model + ". Approval sends this prompt and uses your API account."
      : (isFlutter ? "Local Flutter starter." : "Local project · " + starters[draft.starter][0] + " starter.")) + platforms;
    confirmCreate.textContent = ai ? "Send & create project" : (isFlutter ? "Create Flutter project" : "Create SwiftUI starter");
    const blocker = isFlutter && !flutterSDK ? "Install the Flutter SDK before creating this project."
      : ai && !settings.key ? "Add a key for this provider in Admin to send the prompt." : "";
    confirmCreate.disabled = Boolean(blocker);
    show(reviewStatus, blocker, confirmCreate.disabled);
    show(message, "");
    fields.disabled = false;
    submit.focus();
    review.showModal();
    promptText.scrollTop = 0;
  } catch (error) {
    show(message, error.message, true);
  } finally {
    fields.disabled = quit.disabled;
  }
});
editBrief.addEventListener("click", () => review.close());
review.addEventListener("cancel", event => { if (creating) event.preventDefault(); });
review.addEventListener("close", () => {
  reviewedRequest = undefined;
  promptText.value = "";
});
byID("copy-prompt").addEventListener("click", async () => {
  promptText.focus();
  promptText.select();
  try {
    await navigator.clipboard.writeText(promptText.value);
    show(reviewStatus, "Prompt copied.");
  } catch {
    show(reviewStatus, "Prompt selected. Use your system’s copy shortcut.");
  }
});
confirmCreate.addEventListener("click", async () => {
  if (!reviewedRequest || creating) return;
  const project = reviewedRequest;
  creating = true;
  confirmCreate.disabled = editBrief.disabled = true;
  show(reviewStatus, project.provider ? "Waiting for the model to generate the source…" : "Creating your project…");
  try {
    const result = await request("/api/create", project.provider ? {...project, api_key: profiles[project.provider].key} : project);
    const label = starters[result.starter]?.[0] ?? "SwiftUI";
    review.close();
    show(message, result.kind === "flutter-project"
      ? "Created " + result.suggested_scheme + " for " + result.platforms.map(platform => platformNames[platform]).join(", ") + "."
      : project.provider ? "Created " + result.suggested_scheme + ". Review the generated SwiftUI in Xcode before building."
      : "Created " + result.suggested_scheme + " with the " + label + " starter.");
    await refresh().catch(() => {});
  } catch (error) {
    show(reviewStatus, error.message, true);
  } finally {
    creating = false;
    confirmCreate.disabled = editBrief.disabled = false;
  }
});

folder.addEventListener("click", async () => {
  try { await request("/api/open-folder"); }
  catch (error) { show(message, error.message, true); }
});
quit.addEventListener("click", async () => {
  try {
    await request("/api/quit");
    fields.disabled = folder.disabled = quit.disabled = adminButton.disabled = true;
    for (const profile of Object.values(profiles)) profile.key = "";
    show(message, "AppRivet has quit. You can close this tab.");
  } catch (error) {
    show(message, error.message, true);
  }
});

updateFormat();
if (!session) {
  show(message, "Start AppRivet from your desktop to open a new session.", true);
} else {
  refresh().then(() => {
    fields.disabled = folder.disabled = quit.disabled = adminButton.disabled = false;
    show(message, "");
  }).catch(error => show(message, error.message, true));
}
