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
const planComponents = byID("plan-components");
const format = byID("format");
const admin = byID("admin");
const adminFields = byID("admin-fields");
const adminButton = byID("open-admin");
const provider = byID("provider");
const model = byID("model");
const customModel = byID("custom-model");
const apiKey = byID("api-key");
const keyStatus = byID("key-status");
const clearKey = byID("clear-key");
const appearance = byID("appearance");
const review = byID("prompt-review");
const reviewHeading = byID("review-heading");
const reviewContent = review.querySelector(".review-content");
const reviewPrompts = byID("review-prompts");
const reviewStatus = byID("review-status");
const promptText = byID("prompt-text");
const copyPrompt = byID("copy-prompt");
const confirmCreate = byID("confirm-create");
const editBrief = byID("edit-brief");
const importStatus = byID("import-status");
const openCurrentFolder = byID("open-current-folder");
const generateFromStarter = byID("generate-from-starter");

let reviewedRequest;
let creating = false;
let activeProvider = "openai";
let flutterSDK = false;
let availableFormats = [];
let selectedFormat = "";
let buildSelection;
let building = false;
let loadedProject = "";
let loadedStarter = "";
let openSequence = 0;
let preferComponentPlan = true;
let credentialStorage = false;
let savedKeys = new Set();
let vaultRevision = 0;
const publishPanel = byID("publish-panel");
const publishPlatform = byID("publish-platform");
const publishArtifacts = byID("publish-artifacts");
const trustPublish = byID("trust-publish");
const buildForStore = byID("build-for-store");
const publishStatus = byID("publish-status");
const publishHistoryStatus = byID("publish-history-status");
const storeSettings = byID("store-settings");
const storeSettingsFields = byID("store-settings-fields");
const storeSettingsStatus = byID("store-settings-status");
const storeMetadata = {
  apple_team_id: byID("apple-team-id"),
  apple_issuer_id: byID("apple-issuer-id"),
  apple_key_id: byID("apple-key-id"),
  android_upload_certificate_sha256: byID("android-upload-certificate"),
};
const storeFiles = {
  apple: {input: byID("apple-private-key"), key: "apple_private_key", extension: /\.p8$/i, maxBytes: 8 * 1024},
  google: {input: byID("google-service-account"), key: "google_service_account", extension: /\.json$/i, maxBytes: 24 * 1024},
};
let publishProject;
let publishing = false;
let savedStoreSettings;
let storeSettingsBusy = false;

const singlePromptBriefBytes = 4096;
const briefEncoder = new TextEncoder();
const platformNames = {ios: "iOS", android: "Android", web: "Web", windows: "Windows", macos: "macOS", linux: "Linux"};
const formatNames = {flutter: "Flutter (shared code)", swiftui: "SwiftUI (native iOS)"};
const storeTargets = {
  android: {
    label: "Google Play (Android)",
    output: "Signed Android App Bundle (.aab)",
    setting: "android_upload_certificate_sha256",
    missing: "Save your upload certificate SHA-256 in Store connection settings.",
    steps: [
      "Install Flutter and the Android SDK on this computer.",
      "Set up Play App Signing and configure the project’s Android release build to use your upload keystore.",
      "Save the SHA-256 fingerprint of that upload certificate in Store connection settings.",
      "Build the signed .aab, then upload it to a release in Play Console.",
    ],
  },
  ios: {
    label: "App Store (iOS)",
    output: "Signed iOS archive export (.ipa)",
    setting: "apple_team_id",
    missing: "Save your Apple Team ID in Store connection settings.",
    steps: [
      "Open the project on a Mac with Xcode. For Flutter projects, install Flutter on that Mac too.",
      "Configure App Store distribution signing for the app target in Xcode and make the certificate and provisioning profile available.",
      "Save the Apple Team ID for that signing team in Store connection settings.",
      "Build the signed .ipa, then upload it to App Store Connect with Transporter.",
    ],
  },
};

const providers = {
  openai: {name: "OpenAI", models: [["gpt-6.2-sol", "GPT-6.2 Sol"], ["gpt-6.2-astra", "GPT-6.2 Astra"]]},
  anthropic: {name: "Anthropic", models: [["claude-opus-5-5", "Claude Opus 5.5"], ["claude-fable-5-2", "Claude Fable 5.2"]]},
};
// Session-only keys live in memory. The credential vault exposes only which providers have keys.
const profiles = {
  openai: {model: providers.openai.models[0][0], key: ""},
  anthropic: {model: providers.anthropic.models[0][0], key: ""},
};
const keyPattern = /^[A-Za-z0-9_-]{8,512}$/;

function hasProviderKey(id) {
  return savedKeys.has(id) || Boolean(profiles[id].key);
}

const starters = {
  ai: ["AI-generated SwiftUI", "A model writes the app’s SwiftUI source from your brief. Review the source before building in Xcode."],
  auto: ["Offline starter from brief", "Pick a checklist, saved note or counter template. Other ideas start with a blank SwiftUI screen."],
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
  const needsComponentPlan = briefEncoder.encode(brief.value.trim()).length > singlePromptBriefBytes;
  const ai = Boolean(format.value) && starter.value === "ai";
  byID("generation-settings").hidden = !ai;
  planComponents.disabled = needsComponentPlan;
  planComponents.checked = needsComponentPlan || preferComponentPlan;
  updateLoadedProject();
  byID("generation-help").textContent = needsComponentPlan
    ? "Descriptions over 4,096 UTF-8 bytes use component planning."
    : "Review a component plan before sending the code prompts.";
  if (!format.value) return;
  const isFlutter = format.value === "flutter";
  const [title, description] = isFlutter
    ? ai
      ? ["AI-generated Flutter", "The selected model writes shared Dart code for the platforms you checked."]
      : ["Offline Flutter starter", "A template screen and build files for the platforms you checked."]
    : starters[starter.value];
  byID("starter-title").textContent = title;
  byID("starter-copy").textContent = description;
  byID("output-title").textContent = isFlutter ? "Flutter project" : "Xcode project";
  byID("output-source").textContent = isFlutter
    ? ai && planComponents.checked ? "Shared Dart code in lib/" : "Shared Dart code in lib/main.dart"
    : "Editable SwiftUI source";
  byID("run-heading").textContent = isFlutter ? "Build your app" : "Run in Simulator";
  byID("run-copy").textContent = isFlutter
    ? "Pick a build target beside a saved project. Targets that require another host use the same project folder."
    : "On your Mac, open the project in AppRivet and choose Build & Run.";
  const settings = profiles[activeProvider];
  byID("selected-model").textContent = ai
    ? providers[activeProvider].name + " · " + settings.model + (hasProviderKey(activeProvider) ? "" : " · Add a key in Settings")
    : "Offline starters use template code. Choose Generate with AI to implement your brief.";
}

function showFormats() {
  if (!selectedFormat) selectedFormat = availableFormats[0] || "";
  const options = availableFormats.map(id => new Option(formatNames[id], id));
  if (!availableFormats.includes(selectedFormat)) {
    const placeholder = new Option("Choose a project format", "");
    placeholder.disabled = true;
    options.unshift(placeholder);
  }
  format.replaceChildren(...options);
  format.value = availableFormats.includes(selectedFormat) ? selectedFormat : "";
  byID("format-settings").hidden = availableFormats.length === 0;
  updateFormat();
}

function updateFormat() {
  const isFlutter = format.value === "flutter";
  byID("platform-settings").hidden = !isFlutter;
  byID("bundle-settings").hidden = format.value !== "swiftui";
  byID("creation-method-settings").hidden = !format.value;
  byID("starter-panel").hidden = !format.value;
  byID("build-panel").hidden = !format.value;
  for (const name of ["auto", "checklist", "notes", "counter"]) {
    starter.querySelector('option[value="' + name + '"]').hidden = isFlutter;
  }
  starter.querySelector('option[value="blank"]').textContent = isFlutter ? "Offline Flutter starter" : "Blank";
  if (isFlutter && starter.value !== "ai") starter.value = "blank";
  updateBrief();
}

function updateLoadedProject() {
  const sameName = nameField.value.trim().toLowerCase() === loadedProject.toLowerCase();
  const ai = starter.value === "ai";
  submit.disabled = !format.value || Boolean(loadedProject && sameName) || (ai && !brief.value.trim());
  submit.textContent = ai ? (planComponents.checked ? "Review planning requests" : "Review AI prompt") : "Review offline starter";
  openCurrentFolder.hidden = !loadedProject;
  generateFromStarter.hidden = !loadedStarter || loadedStarter === "ai";
  generateFromStarter.disabled = !format.value || !brief.value.trim();
  for (const button of projects.querySelectorAll("[data-project-name]")) {
    button.setAttribute("aria-current", String(button.dataset.projectName === loadedProject));
  }
}

async function reopenProject(name) {
  if (publishing) return;
  closePublish();
  const sequence = ++openSequence;
  show(message, "Reopening " + name + "…");
  try {
    const {project, settings} = await request("/api/project", {name});
    if (sequence !== openSequence) return;
    loadedProject = name;
    loadedStarter = project.starter ?? "";
    nameField.value = settings.name;
    brief.value = settings.brief || "";
    byID("bundle").value = settings.bundle_id || "";
    selectedFormat = settings.format;
    showFormats();
    starter.value = settings.starter;
    for (const input of form.querySelectorAll('input[name="platform"]')) {
      input.checked = settings.platforms.includes(input.value);
    }
    if (Object.hasOwn(providers, settings.provider) && /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(settings.model || "")) {
      activeProvider = settings.provider;
      profiles[activeProvider].model = settings.model;
    }
    show(importStatus, "");
    updateBrief();
    show(message, "Opened " + name + (loadedStarter && loadedStarter !== "ai"
      ? ". This project was created as an offline starter."
      : format.value ? ". Use a different project name to create a new copy." : "."));
    brief.focus();
  } catch (error) {
    if (sequence === openSequence) show(message, error.message, true);
  }
}

nameField.addEventListener("input", updateLoadedProject);
generateFromStarter.addEventListener("click", () => {
  if (nameField.value.trim().toLowerCase() === loadedProject.toLowerCase()) {
    const candidate = loadedProject.slice(0, 48) + "AI";
    nameField.value = candidate.toLowerCase() === loadedProject.toLowerCase() ? loadedProject.slice(0, 47) + "App" : candidate;
    byID("bundle").value = "";
  }
  starter.value = "ai";
  preferComponentPlan = true;
  updateBrief();
  show(message, "Review the brief and planning requests to generate a new project with AI.");
  nameField.focus();
});
openCurrentFolder.addEventListener("click", async () => {
  try { await request("/api/open-project-folder", {name: loadedProject}); }
  catch (error) { show(message, error.message, true); }
});

async function request(path, body) {
  const headers = {"X-AppRivet-Session": session};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(path, {
    method: path === "/api/state" || (path === "/api/publish/settings" && body === undefined) ? "GET" : "POST",
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return;
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || "AppRivet could not complete this action.");
  return result;
}

async function refresh() {
  const revision = vaultRevision;
  const state = await request("/api/state");
  if (revision === vaultRevision) {
    credentialStorage = state.credential_storage === true;
    savedKeys = new Set(state.saved_keys ?? []);
    if (admin.open && !adminFields.disabled) refreshKeyStatus();
  }
  flutterSDK = state.flutter_sdk;
  availableFormats = state.project_formats.filter(id => Object.hasOwn(formatNames, id));
  byID("flutter-sdk-help").hidden = flutterSDK;
  showFormats();
  byID("workspace-path").textContent = state.workspace;
  byID("empty").hidden = state.projects.length !== 0;
  byID("project-instructions").hidden = state.projects.length === 0;
  const rows = document.createDocumentFragment();
  for (const name of state.projects) {
    const row = document.createElement("li");
    const rowHeader = document.createElement("div");
    rowHeader.className = "project-row-header";
    const open = document.createElement("button");
    open.className = "project-name";
    open.type = "button";
    open.dataset.projectName = name;
    open.textContent = name;
    open.setAttribute("aria-current", String(name === loadedProject));
    open.addEventListener("click", () => reopenProject(name));
    const flutter = state.flutter_projects.find(project => project.name === name);
    const publish = document.createElement("button");
    publish.className = "quiet";
    publish.type = "button";
    publish.dataset.publishProject = name;
    publish.textContent = "Publish";
    publish.setAttribute("aria-label", "Publish " + name);
    publish.setAttribute("aria-controls", "publish-panel");
    publish.setAttribute("aria-expanded", String(publishProject?.name === name));
    publish.addEventListener("click", () => openPublish(name, flutter));
    rowHeader.append(open, publish);
    row.append(rowHeader);
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
  if (publishing) return;
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

function updatePublishButtons() {
  for (const button of projects.querySelectorAll("[data-publish-project]")) {
    button.setAttribute("aria-expanded", String(button.dataset.publishProject === publishProject?.name));
    button.disabled = publishing;
  }
  byID("close-publish").disabled = publishing;
}

function closePublish() {
  if (!publishProject) return;
  publishProject = undefined;
  publishPanel.hidden = true;
  form.hidden = byID("create-intro").hidden = false;
  updatePublishButtons();
}

function showPublishTarget() {
  if (!publishProject) return;
  const target = storeTargets[publishPlatform.value];
  const steps = target?.steps.map(text => {
    const step = document.createElement("li");
    step.textContent = text;
    return step;
  }) ?? [];
  byID("publish-steps").replaceChildren(...steps);
  byID("publish-checklist").hidden = !target;
  byID("publish-target-help").textContent = target?.output ?? "";
  const setting = target && publishProject.settings?.[target.setting];
  const readiness = !target || setting ? "" : publishProject.settings ? target.missing
    : publishProject.settingsFailed ? "Open Store connection settings to load your signing settings."
    : "Checking store signing settings…";
  show(byID("publish-readiness"), readiness, publishProject.settingsFailed);
  buildForStore.disabled = !setting || !trustPublish.checked || publishing;
}

function showArtifacts(selection) {
  const rows = selection.artifacts.map(artifact => {
    const row = document.createElement("li");
    const name = document.createElement("strong");
    name.textContent = artifact.artifact.split(/[\\/]/).at(-1);
    const details = document.createElement("p");
    details.className = "field-help";
    details.textContent = platformNames[artifact.platform] + " / " + artifact.size.toLocaleString() + " bytes";
    const checksum = document.createElement("p");
    checksum.className = "field-help artifact-checksum";
    checksum.textContent = "SHA-256 " + artifact.sha256;
    const reveal = document.createElement("button");
    reveal.type = "button";
    reveal.className = "quiet";
    reveal.textContent = "Open artifact folder";
    reveal.setAttribute("aria-label", "Open artifact folder for " + name.textContent);
    reveal.addEventListener("click", async () => {
      reveal.disabled = true;
      show(publishHistoryStatus, "");
      try {
        await request("/api/publish/open-artifact", {name: selection.name, sha256: artifact.sha256});
      } catch (error) {
        if (publishProject === selection) show(publishHistoryStatus, error.message, true);
      } finally {
        reveal.disabled = false;
      }
    });
    row.append(name, details, checksum, reveal);
    return row;
  });
  publishArtifacts.replaceChildren(...rows);
  show(publishHistoryStatus, rows.length ? "" : "Signed releases you build will appear here.");
}

async function loadPublishHistory(selection) {
  try {
    const artifacts = await request("/api/publish/history", {name: selection.name});
    if (publishProject !== selection) return;
    const known = new Set(selection.artifacts.map(artifact => artifact.sha256));
    selection.artifacts.push(...(artifacts ?? []).filter(artifact => !known.has(artifact.sha256)));
    showArtifacts(selection);
  } catch (error) {
    if (publishProject === selection && !selection.artifacts.length) show(publishHistoryStatus, error.message, true);
  }
}

async function loadPublishSettings(selection) {
  try {
    const settings = await request("/api/publish/settings");
    if (publishProject !== selection || selection.settings) return;
    selection.settings = settings;
  } catch {
    if (publishProject !== selection || selection.settings) return;
    selection.settingsFailed = true;
  }
  showPublishTarget();
}

function openPublish(name, flutter) {
  if (publishing) return;
  if (publishProject?.name === name) {
    closePublish();
    return;
  }
  openSequence++;
  const selection = {name, artifacts: []};
  const options = [];
  const unavailable = [];
  const mac = availableFormats.includes("swiftui");
  if (flutter?.platforms.includes("android")) {
    if (flutterSDK && flutter.build_targets.includes("android")) {
      options.push(new Option(storeTargets.android.label, "android"));
    } else {
      unavailable.push("Install the Flutter SDK and reopen AppRivet to build for Google Play.");
    }
  }
  if (!flutter || flutter.platforms.includes("ios")) {
    if (!mac) {
      unavailable.push("Open this project on a Mac with Xcode to sign for the App Store.");
    } else if (!flutter || (flutterSDK && flutter.build_targets.includes("ios"))) {
      options.push(new Option(storeTargets.ios.label, "ios"));
    } else {
      unavailable.push("Install the Flutter SDK and reopen AppRivet to build for the App Store.");
    }
  }
  if (!options.length && !unavailable.length) unavailable.push("This project needs an Android or iOS target for a store build.");
  publishProject = selection;
  publishPlatform.replaceChildren(...(options.length ? options : [new Option("No store target available", "")]));
  publishPlatform.disabled = !options.length;
  trustPublish.checked = false;
  form.hidden = byID("create-intro").hidden = true;
  publishPanel.hidden = false;
  const heading = byID("publish-heading");
  heading.textContent = "Publish " + name;
  byID("publish-unavailable").textContent = unavailable.join(" ");
  show(publishStatus, "");
  publishArtifacts.replaceChildren();
  show(publishHistoryStatus, "Loading signed artifacts…");
  updatePublishButtons();
  showPublishTarget();
  heading.focus();
  void loadPublishHistory(selection);
  if (options.length) void loadPublishSettings(selection);
}

byID("close-publish").addEventListener("click", () => {
  if (publishProject) void reopenProject(publishProject.name);
});
publishPlatform.addEventListener("change", () => {
  trustPublish.checked = false;
  show(publishStatus, "");
  showPublishTarget();
});
trustPublish.addEventListener("change", showPublishTarget);
buildForStore.addEventListener("click", async () => {
  if (buildForStore.disabled || !publishProject) return;
  const selection = publishProject;
  const platform = publishPlatform.value;
  publishing = true;
  byID("publish-build-fields").disabled = true;
  updatePublishButtons();
  showPublishTarget();
  show(publishStatus, "Building the signed " + platformNames[platform] + " release locally…");
  try {
    const artifact = await request("/api/publish/build", {name: selection.name, platform, trusted: true});
    selection.artifacts = [artifact, ...selection.artifacts.filter(item => item.sha256 !== artifact.sha256)];
    showArtifacts(selection);
    show(publishStatus, "Signed release ready. Open its folder when you’re ready to upload it.");
  } catch (error) {
    show(publishStatus, error.message, true);
  } finally {
    publishing = false;
    byID("publish-build-fields").disabled = false;
    updatePublishButtons();
    showPublishTarget();
  }
});

function updateStoreControls() {
  const ready = Boolean(savedStoreSettings) && !storeSettingsBusy;
  storeSettingsFields.disabled = !ready;
  byID("save-store-settings").disabled = !ready;
  byID("close-store-settings").disabled = storeSettingsBusy;
  for (const [store, {input}] of Object.entries(storeFiles)) {
    const hasKey = Boolean(savedStoreSettings?.["has_" + store + "_key"]);
    const canStore = Boolean(savedStoreSettings?.credential_storage);
    input.disabled = !canStore;
    byID("remove-" + store + "-setting").hidden = !canStore || !hasKey;
    byID("test-" + store + "-connection").disabled = !canStore || !hasKey || (store === "apple" && !savedStoreSettings.apple_key_id);
    byID(store + "-credential-status").textContent = hasKey ? "Key saved. Choose a file to replace it." : "No key saved.";
  }
}

function showStoreSettings(settings) {
  savedStoreSettings = settings;
  for (const [key, input] of Object.entries(storeMetadata)) input.value = settings[key] ?? "";
  for (const [store, {input}] of Object.entries(storeFiles)) {
    input.value = "";
    byID("remove-" + store + "-key").checked = false;
    show(byID(store + "-connection-status"), "");
  }
  byID("store-credential-storage").textContent = settings.credential_storage
    ? "AppRivet stores imported keys in your operating system’s credential vault. Connection tests use saved credentials."
    : "Protected credential storage is unavailable. You can still save identifiers for local signing.";
  if (publishProject) {
    publishProject.settings = settings;
    publishProject.settingsFailed = false;
    showPublishTarget();
  }
  updateStoreControls();
}

async function openStoreSettings(fromAdmin) {
  if (storeSettingsBusy || storeSettings.open || (fromAdmin && adminFields.disabled)) return;
  byID("close-store-settings").textContent = fromAdmin ? "Back to Settings" : "Close";
  savedStoreSettings = undefined;
  storeSettingsBusy = true;
  byID("store-credential-storage").textContent = "";
  storeSettingsFields.hidden = true;
  updateStoreControls();
  show(storeSettingsStatus, "Loading store settings…");
  storeSettings.showModal();
  byID("store-settings-heading").focus();
  try {
    showStoreSettings(await request("/api/publish/settings"));
    storeSettingsFields.hidden = false;
    show(storeSettingsStatus, "");
  } catch (error) {
    show(storeSettingsStatus, error.message, true);
  } finally {
    storeSettingsBusy = false;
    updateStoreControls();
  }
}

byID("admin-open-stores").addEventListener("click", () => openStoreSettings(true));
byID("publish-open-settings").addEventListener("click", () => openStoreSettings(false));
byID("close-store-settings").addEventListener("click", () => storeSettings.close());
storeSettings.addEventListener("cancel", event => { if (storeSettingsBusy) event.preventDefault(); });
storeSettings.addEventListener("close", () => {
  for (const {input} of Object.values(storeFiles)) input.value = "";
});

for (const [store, {input}] of Object.entries(storeFiles)) {
  const remove = byID("remove-" + store + "-key");
  input.addEventListener("change", () => { if (input.files.length) remove.checked = false; });
  remove.addEventListener("change", () => { if (remove.checked) input.value = ""; });
  byID("test-" + store + "-connection").addEventListener("click", async () => {
    if (storeSettingsBusy) return;
    const status = byID(store + "-connection-status");
    storeSettingsBusy = true;
    updateStoreControls();
    show(status, "Testing saved credentials…");
    try {
      const result = await request("/api/publish/connection", {store});
      show(status, result.message);
    } catch (error) {
      show(status, error.message, true);
    } finally {
      storeSettingsBusy = false;
      updateStoreControls();
    }
  });
}

byID("store-settings-form").addEventListener("submit", async event => {
  event.preventDefault();
  if (storeSettingsBusy || !savedStoreSettings) return;
  storeSettingsBusy = true;
  updateStoreControls();
  show(storeSettingsStatus, "Saving store settings…");
  const body = Object.fromEntries(Object.entries(storeMetadata).map(([key, input]) => [key, input.value.trim()]));
  try {
    if (savedStoreSettings.credential_storage) {
      for (const [store, {input, key, extension, maxBytes}] of Object.entries(storeFiles)) {
        if (input.files[0]) body[key] = await readTextFile(input.files[0], extension, maxBytes);
        if (byID("remove-" + store + "-key").checked) body["remove_" + store + "_key"] = true;
      }
    }
    showStoreSettings(await request("/api/publish/settings", body));
    show(storeSettingsStatus, "Store settings saved.");
  } catch (error) {
    show(storeSettingsStatus, error.message, true);
  } finally {
    for (const {input} of Object.values(storeFiles)) input.value = "";
    storeSettingsBusy = false;
    updateStoreControls();
  }
});

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
  byID("credential-storage").textContent = credentialStorage
    ? "AppRivet saves one key per provider in your operating system’s credential vault."
    : "API keys are available for this session only. You can set one key per provider.";
  const id = provider.value;
  show(keyStatus, savedKeys.has(id) ? "Key saved for this provider."
    : hasProviderKey(id) ? "Key set for this session." : "No key set for this provider.");
  clearKey.disabled = !hasProviderKey(id);
}

async function updateProviderKey(id, key) {
  if (credentialStorage && (key || savedKeys.has(id))) {
    adminFields.disabled = true;
    show(keyStatus, key ? "Saving the key…" : "Removing the key…");
    try {
      if (key) {
        await request("/api/provider-key", {provider: id, key});
        savedKeys.add(id);
      } else {
        await request("/api/remove-provider-key", {provider: id});
        savedKeys.delete(id);
      }
      vaultRevision++;
      profiles[id].key = "";
    } finally {
      adminFields.disabled = false;
    }
  } else {
    profiles[id].key = key;
  }
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
  appearance.value = document.documentElement.dataset.theme;
  showProviderSettings();
  admin.showModal();
});
admin.addEventListener("close", () => { if (!admin.open) apiKey.value = ""; });
admin.addEventListener("cancel", event => { if (adminFields.disabled) event.preventDefault(); });
byID("close-admin").addEventListener("click", () => admin.close());
provider.addEventListener("change", showProviderSettings);
model.addEventListener("change", updateCustomField);
clearKey.addEventListener("click", async () => {
  if (adminFields.disabled) return;
  try {
    await updateProviderKey(provider.value, "");
    apiKey.value = "";
    refreshKeyStatus();
    updateBrief();
  } catch (error) {
    show(keyStatus, error.message, true);
  }
});
watchFile(byID("import-key"), byID("key-file"), async file => {
  const selected = provider.value;
  try {
    const key = (await readTextFile(file, /\.txt$/i, 1024)).trim();
    if (!keyPattern.test(key)) throw new Error("Choose a text file containing just one API key.");
    if (!admin.open || adminFields.disabled || provider.value !== selected) return;
    apiKey.value = key;
    show(keyStatus, "Key loaded. Save settings to use it.");
  } catch (error) {
    if (admin.open && !adminFields.disabled && provider.value === selected) show(keyStatus, error.message, true);
  }
});
byID("admin-form").addEventListener("submit", async event => {
  event.preventDefault();
  if (adminFields.disabled) return;
  const selected = provider.value;
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
  if (key) {
    try {
      await updateProviderKey(selected, key);
    } catch (error) {
      show(keyStatus, error.message.replaceAll(key, "[redacted]"), true);
      apiKey.focus();
      return;
    }
  }
  activeProvider = selected;
  profiles[activeProvider].model = modelID;
  const theme = appearance.value;
  document.documentElement.dataset.theme = theme;
  byID("brand-mark").src = theme === "light" ? "/apprivet-mark-dark.png" : "/apprivet-mark-light.png";
  admin.close();
  updateBrief();
});

brief.addEventListener("input", updateBrief);
starter.addEventListener("change", updateBrief);
planComponents.addEventListener("change", () => {
  preferComponentPlan = planComponents.checked;
  updateBrief();
});
format.addEventListener("change", () => {
  selectedFormat = format.value;
  updateFormat();
});
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

function displayStagedReview(draft) {
  const plan = draft.plan;
  reviewHeading.textContent = plan ? "Review component plan" : "Review planning prompts";
  byID("review-budget").textContent = draft.request_count.toLocaleString()
    + (draft.request_count === 1 ? " request, up to " : " requests, up to ")
    + draft.max_output_tokens.toLocaleString() + " output tokens in this step.";
  byID("review-synthesis").hidden = !draft.uses_synthesis;
  byID("review-plan").hidden = !plan;
  byID("review-architecture").textContent = plan?.architecture ?? "";
  byID("review-components").replaceChildren(...(plan?.components ?? []).map(component => {
    const item = document.createElement("li");
    const title = document.createElement("strong");
    title.textContent = component.title;
    const scope = document.createElement("p");
    scope.textContent = component.scope;
    item.append(title, scope);
    return item;
  }));
  byID("review-prompts-heading").hidden = !plan;
  reviewPrompts.replaceChildren(...draft.prompts.map((entry, index) => {
    const details = document.createElement("details");
    details.open = index === 0;
    const summary = document.createElement("summary");
    summary.textContent = entry.title;
    const caption = document.createElement("span");
    caption.textContent = (entry.file ? entry.file + " (" : "")
      + "Up to " + entry.max_output_tokens.toLocaleString() + " output tokens" + (entry.file ? ")" : "");
    summary.append(caption);
    const prompt = document.createElement("pre");
    prompt.tabIndex = 0;
    prompt.textContent = entry.prompt;
    details.append(summary, prompt);
    return details;
  }));
  confirmCreate.textContent = plan ? "Approve plan & generate" : "Send to plan";
}

form.addEventListener("submit", async event => {
  event.preventDefault();
  if (fields.disabled || submit.disabled) return;
  const ai = starter.value === "ai";
  const staged = ai && planComponents.checked;
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
    const draft = await request(staged ? "/api/plan/review" : "/api/review", project);
    reviewedRequest = {
      project: ai ? {...project, approval: draft.approval} : project,
      stage: staged ? "planning" : "single",
    };
    byID("staged-review").hidden = !staged;
    byID("single-prompt").hidden = copyPrompt.hidden = staged;
    if (staged) {
      displayStagedReview(draft);
    } else {
      reviewHeading.textContent = ai ? "Review AI prompt" : "Review offline starter";
      promptText.value = draft.prompt;
      confirmCreate.textContent = ai ? "Send & generate project" : "Create offline starter";
    }
    const platforms = isFlutter ? " Flutter targets are " + (draft.platforms ?? project.platforms).map(platform => platformNames[platform]).join(", ") + "." : "";
    byID("review-target").textContent = (ai
      ? providers[draft.provider ?? project.provider].name + " (" + (draft.model ?? project.model)
        + "). Approval sends " + (staged ? "the requests for this step" : "this prompt") + " using your API account."
      : "This project uses an offline template. Your brief is saved with the project.") + platforms;
    const blocker = isFlutter && !flutterSDK ? "Install the Flutter SDK before creating this project."
      : ai && !hasProviderKey(project.provider) ? "Add a key for this provider in Settings before sending." : "";
    confirmCreate.disabled = Boolean(blocker);
    show(reviewStatus, blocker, confirmCreate.disabled);
    show(message, "");
    fields.disabled = false;
    submit.focus();
    review.showModal();
    review.scrollTop = 0;
    reviewContent.scrollTop = 0;
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
  if (review.open) return;
  reviewedRequest = undefined;
  promptText.value = "";
  reviewPrompts.replaceChildren();
  byID("review-components").replaceChildren();
  byID("review-architecture").textContent = "";
});
copyPrompt.addEventListener("click", async () => {
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
  if (!reviewedRequest || creating || confirmCreate.disabled) return;
  const {project, stage} = reviewedRequest;
  const key = project.provider ? profiles[project.provider].key : "";
  const approvedRequest = key ? {...project, api_key: key} : project;
  creating = true;
  confirmCreate.disabled = editBrief.disabled = true;
  show(reviewStatus, stage === "planning" ? "Waiting for the model to plan the components…"
    : project.provider ? "Waiting for the model to generate the source…" : "Creating your project…");
  try {
    if (stage === "planning") {
      const draft = await request("/api/plan/draft", approvedRequest);
      displayStagedReview(draft);
      reviewedRequest = {project: {...project, approval: draft.approval, plan: draft.plan}, stage: "generation"};
      show(reviewStatus, "");
      review.scrollTop = 0;
      reviewContent.scrollTop = 0;
      reviewHeading.focus();
      return;
    }
    const result = await request(stage === "generation" ? "/api/plan/create" : "/api/create", approvedRequest);
    review.close();
    loadedProject = result.suggested_scheme;
    loadedStarter = result.starter;
    updateLoadedProject();
    show(message, result.starter === "ai"
      ? "AI generated the source for " + result.suggested_scheme + ". Review the source before building."
      : "Created " + result.suggested_scheme + " as an offline starter.");
    await refresh().catch(() => {});
  } catch (error) {
    show(reviewStatus, key ? error.message.replaceAll(key, "[redacted]") : error.message, true);
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
