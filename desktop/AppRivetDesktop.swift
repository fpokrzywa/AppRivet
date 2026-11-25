import AppKit
import Darwin
import SwiftUI
import UniformTypeIdentifiers

private enum AIProvider: String, CaseIterable, Identifiable, Hashable {
    case openai, anthropic

    var id: String { rawValue }
    var label: String { self == .openai ? "OpenAI" : "Anthropic" }
    var models: [(id: String, label: String)] {
        switch self {
        case .openai: return [("gpt-6.2-sol", "GPT-6.2 Sol"), ("gpt-6.2-astra", "GPT-6.2 Astra")]
        case .anthropic: return [("claude-opus-5-5", "Claude Opus 5.5"), ("claude-fable-5-2", "Claude Fable 5.2")]
        }
    }
}

private struct AIProfile {
    var model: String
    var key = ""
}

private let appPlatforms: [(id: String, label: String)] = [
    ("ios", "iOS"), ("android", "Android"), ("web", "Web"),
    ("windows", "Windows"), ("macos", "macOS"), ("linux", "Linux"),
]

private func platformLabel(_ id: String) -> String {
    appPlatforms.first { $0.id == id }?.label ?? id
}

@MainActor
private enum DesktopBrand {
    private static let artwork = Bundle.main.url(forResource: "AppRivetBlueprint", withExtension: "png")
        .flatMap { NSImage(contentsOf: $0) }

    @ViewBuilder static var mark: some View {
        if let artwork {
            Image(nsImage: artwork)
                .resizable()
                .frame(width: 132, height: 63)
                .frame(width: 46, height: 46)
                .clipped()
                .clipShape(RoundedRectangle(cornerRadius: 9))
                .overlay { RoundedRectangle(cornerRadius: 9).strokeBorder(.white.opacity(0.18)) }
                .accessibilityHidden(true)
        }
    }

    static var backdrop: some View {
        GeometryReader { geometry in
            ZStack {
                Color(red: 0.063, green: 0.094, blue: 0.122)
                RadialGradient(colors: [Color(red: 0.114, green: 0.278, blue: 0.282), .clear],
                               center: UnitPoint(x: 0.08, y: 0), startRadius: 0, endRadius: geometry.size.width * 0.65)
                RadialGradient(colors: [Color(red: 0.149, green: 0.224, blue: 0.29), .clear],
                               center: UnitPoint(x: 0.88, y: 0.30), startRadius: 0, endRadius: geometry.size.width * 0.54)
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }
}

@MainActor
private func chooseTextFile(title: String, extensions: [String]) -> URL? {
    let panel = NSOpenPanel()
    panel.title = title
    panel.prompt = "Import"
    panel.canChooseDirectories = false
    panel.allowedContentTypes = extensions.compactMap { UTType(filenameExtension: $0) }
    guard panel.runModal() == .OK else { return nil }
    return panel.url
}

private func readTextFile(_ url: URL, extensions: [String], maxBytes: Int? = nil) throws -> String {
    let values = try url.resourceValues(forKeys: [.isRegularFileKey])
    guard values.isRegularFile == true, extensions.contains(url.pathExtension.lowercased()) else {
        throw DesktopFailure(message: "Choose a file in a supported format.")
    }
    let file = try FileHandle(forReadingFrom: url)
    defer { try? file.close() }
    let bytes: Data
    if let maxBytes {
        bytes = try file.read(upToCount: maxBytes + 1) ?? Data()
        guard bytes.count <= maxBytes else { throw DesktopFailure(message: "This file is too large.") }
    } else {
        bytes = try file.readToEnd() ?? Data()
    }
    guard var text = String(data: bytes, encoding: .utf8) else {
        throw DesktopFailure(message: "Choose a file saved as UTF-8 plain text.")
    }
    if text.hasPrefix("\u{feff}") { text.removeFirst() }
    return text.replacingOccurrences(of: "\r\n", with: "\n").replacingOccurrences(of: "\r", with: "\n")
}

private func importBrief(_ url: URL) throws -> String {
    let text = try readTextFile(url, extensions: ["txt", "md", "markdown"])
    guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
          !text.unicodeScalars.contains(where: { $0.properties.generalCategory == .control && $0.value != 9 && $0.value != 10 }) else {
        throw DesktopFailure(message: "Choose a file containing plain text or Markdown.")
    }
    return text
}

@main
struct AppRivetDesktop: App {
    init() {
        let arguments = CommandLine.arguments
        guard arguments.dropFirst().first == "--smoke-test" else { return }
        do {
            guard arguments.count == 3 else { throw DesktopFailure(message: "Smoke test requires a parent folder.") }
            guard Bundle.main.url(forResource: "AppRivetBlueprint", withExtension: "png") != nil else {
                throw DesktopFailure(message: "The AppRivet blueprint artwork is missing from the package.")
            }
            let workflow = try DesktopWorkflow.bundled()
            let parent = URL(fileURLWithPath: arguments[2], isDirectory: true)
            let file = parent.appendingPathComponent("TripBrief.md")
            let brief = "A checklist for a weekend trip." + String(repeating: "\nKeep track of gear.", count: 120_000)
            try Data(brief.utf8).write(to: file, options: .withoutOverwriting)
            let imported = try importBrief(file)
            let aiDraft = try workflow.review(name: "DesktopAI", bundleID: "", brief: imported, starter: "ai",
                                              provider: "anthropic", model: "claude-opus-5-5")
            guard aiDraft.prompt.contains(brief), aiDraft.approval?.count == 64, aiDraft.provider == "anthropic" else {
                throw DesktopFailure(message: "The AI prompt review did not preserve the imported brief and target.")
            }
            let sharedDraft = try workflow.review(name: "DesktopShared", bundleID: "", brief: imported, starter: "ai",
                                                  provider: "anthropic", model: "claude-opus-5-5", format: "flutter", platforms: ["web", "linux"])
            guard sharedDraft.platforms == ["web", "linux"], sharedDraft.format == "flutter",
                  sharedDraft.prompt.contains("lib/main.dart") else {
                throw DesktopFailure(message: "The shared project review did not preserve the selected Flutter platforms.")
            }
            let draft = try workflow.review(name: "DesktopSmoke", bundleID: "org.example.apprivetdesktop", brief: imported,
                                            starter: "auto", provider: "", model: "")
            Self.smokeProgress("Creating the SwiftUI test project…")
            let created = try workflow.create(draft: draft, parent: parent)
            guard created.starter == "checklist" else { throw DesktopFailure(message: "The app brief did not select the checklist starter.") }
            let project = try workflow.inspect(URL(fileURLWithPath: created.root, isDirectory: true))
            let opened = try workflow.open(URL(fileURLWithPath: created.root, isDirectory: true))
            guard opened.settings.brief == brief, opened.settings.bundleID == "org.example.apprivetdesktop",
                  opened.settings.starter == "auto" else {
                throw DesktopFailure(message: "The saved project form did not restore the original settings.")
            }
            Self.smokeProgress("Discovering an iPhone simulator…")
            guard let simulator = try workflow.simulators().first(where: { $0.name.hasPrefix("iPhone") }) else {
                throw DesktopFailure(message: "An iPhone simulator is unavailable.")
            }
            let result = try workflow.buildAndRun(project: project, simulator: simulator, trusted: true, showSimulator: false) {
                Self.smokeProgress($0)
            }
            guard result.bundleIdentifier == "org.example.apprivetdesktop" else {
                throw DesktopFailure(message: "The wrong product was launched.")
            }
            print("AppRivet desktop created, built, installed and launched \(result.bundleIdentifier)")
            exit(0)
        } catch {
            let details = (error as? DesktopFailure)?.details ?? ""
            FileHandle.standardError.write(Data("\(error.localizedDescription)\n\(details)\n".utf8))
            exit(1)
        }
    }

    private nonisolated static func smokeProgress(_ message: String) {
        FileHandle.standardOutput.write(Data("\(message)\n".utf8))
    }

    var body: some Scene {
        Window("AppRivet", id: "main") {
            DesktopView()
        }
        .defaultSize(width: 680, height: 740)
    }
}

@MainActor
private final class DesktopModel: ObservableObject {
    @Published var name = "MyFirstApp"
    @Published var brief = ""
    @Published var starter = "blank"
    @Published var format = "flutter"
    @Published var platforms: Set<String> = ["ios", "android", "web"]
    @Published var buildTarget = "web"
    @Published var bundleID = ""
    @Published var parent: URL?
    @Published var selectedSimulator: String?
    @Published var trusted = false
    @Published var draft: DesktopDraft?
    @Published private(set) var selectedProvider = AIProvider.openai
    @Published private(set) var briefMessage = ""
    @Published private(set) var profiles: [AIProvider: AIProfile] = [
        .openai: AIProfile(model: AIProvider.openai.models[0].id),
        .anthropic: AIProfile(model: AIProvider.anthropic.models[0].id),
    ]
    @Published private(set) var project: DesktopProject?
    @Published private(set) var simulators: [DesktopSimulator] = []
    @Published private(set) var loadingSimulators = false
    @Published private(set) var simulatorMessage = "No iOS simulators available"
    @Published private(set) var activity: String?
    @Published private(set) var status = "Choose a folder to create your first app."
    @Published private(set) var errorMessage: String?
    @Published private(set) var log = ""
    @Published private(set) var buildArtifact: URL?

    var busy: Bool { activity != nil }
    var canCreate: Bool {
        guard let parent else { return false }
        let projectName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !projectName.isEmpty else { return false }
        if let project {
            let root = URL(fileURLWithPath: project.root, isDirectory: true)
            let sameFolder = root.deletingLastPathComponent().standardizedFileURL == parent.standardizedFileURL
            if sameFolder && root.lastPathComponent.caseInsensitiveCompare(projectName) == .orderedSame { return false }
        }
        return true
    }
    var isFlutterProject: Bool { project?.kind == "flutter-project" }
    var localTargets: [String] { (project?.platforms ?? []).filter { ["ios", "android", "web", "macos"].contains($0) } }
    var canRun: Bool {
        guard project != nil, trusted else { return false }
        return isFlutterProject && buildTarget != "ios" ? localTargets.contains(buildTarget) : selectedSimulator != nil
    }

    func chooseBrief() {
        guard let file = chooseTextFile(title: "Import app brief", extensions: ["txt", "md", "markdown"]) else { return }
        do {
            brief = try importBrief(file)
            briefMessage = "Imported \(file.lastPathComponent). You can edit it above."
        } catch {
            briefMessage = error.localizedDescription
        }
    }

    func saveSettings(provider: AIProvider, model: String, key: String) {
        profiles[provider] = AIProfile(model: model, key: key.isEmpty ? profiles[provider]?.key ?? "" : key)
        selectedProvider = provider
    }

    func removeKey(for provider: AIProvider) {
        profiles[provider]?.key = ""
    }

    func chooseParent() {
        let panel = NSOpenPanel()
        panel.title = "Choose a location for your new app"
        panel.prompt = "Choose Folder"
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.canCreateDirectories = true
        if panel.runModal() == .OK { parent = panel.url }
    }

    func openProject() {
        let panel = NSOpenPanel()
        panel.title = "Open an AppRivet project"
        panel.message = "Choose the project folder or its .xcodeproj file."
        panel.canChooseDirectories = true
        panel.canChooseFiles = true
        guard panel.runModal() == .OK, let url = panel.url else { return }
        perform("Opening project…", work: { try $0.open(url) }, done: restoreProject)
    }

    private func restoreProject(_ opened: DesktopOpened) {
        let settings = opened.settings
        name = settings.name
        brief = settings.brief ?? ""
        briefMessage = ""
        bundleID = settings.bundleID ?? ""
        format = settings.format
        platforms = Set(settings.platforms)
        starter = settings.starter
        if let provider = settings.provider.flatMap(AIProvider.init(rawValue:)),
           let model = settings.model,
           model.range(of: #"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}\z"#, options: .regularExpression) != nil {
            selectedProvider = provider
            profiles[provider]?.model = model
        }
        parent = URL(fileURLWithPath: opened.project.root, isDirectory: true).deletingLastPathComponent()
        selectProject(opened.project)
        status = "Opened \(settings.name). Use a different project name or save folder to create a new copy."
    }

    func reviewProject() {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let format = self.format
        let bundleID = format == "flutter" ? "" : bundleID.trimmingCharacters(in: .whitespacesAndNewlines)
        let brief = self.brief
        let starter = self.starter
        let provider = selectedProvider
        let model = profiles[provider]?.model ?? ""
        let platforms = format == "flutter" ? appPlatforms.map { $0.id }.filter { self.platforms.contains($0) } : ["ios"]
        perform("Validating the app brief…", work: {
            try $0.review(name: name, bundleID: bundleID, brief: brief, starter: starter, provider: provider.rawValue, model: model,
                          format: format, platforms: platforms)
        }) { self.draft = $0 }
    }

    func key(for draft: DesktopDraft) -> String? {
        guard let provider = draft.provider.flatMap(AIProvider.init(rawValue:)),
              let key = profiles[provider]?.key, !key.isEmpty else { return nil }
        return key
    }

    func createProject(_ draft: DesktopDraft) {
        guard let parent else { return }
        let key = key(for: draft)
        guard !draft.isAI || key != nil else { return }
        self.draft = nil
        perform(draft.isAI ? "Waiting for the model to generate the source…" : "Creating \(draft.name)…", work: {
            try $0.create(draft: draft, parent: parent, apiKey: key)
        }, done: selectProject)
    }

    private func selectProject(_ project: DesktopProject) {
        self.project = project
        trusted = false
        buildArtifact = nil
        if isFlutterProject {
            buildTarget = localTargets.contains("web") ? "web" : localTargets.first ?? ""
            status = "\(project.suggestedScheme) is ready for source review and building."
        } else if project.starter == "ai" {
            status = "\(project.suggestedScheme) is ready for source review in Xcode."
        } else if let starter = project.starter {
            status = "\(project.suggestedScheme) was created with the \(starter) starter."
        } else {
            status = "\(project.suggestedScheme) is ready to build."
        }
    }

    func refreshSimulators() {
        guard !loadingSimulators else { return }
        loadingSimulators = true
        simulatorMessage = "Looking for iOS simulators…"
        Task {
            defer { loadingSimulators = false }
            do {
                let devices = try await Task.detached(priority: .userInitiated) { try DesktopWorkflow.bundled().simulators() }.value
                self.simulators = devices
                if !devices.contains(where: { $0.id == self.selectedSimulator }) {
                    self.selectedSimulator = (devices.first { $0.name.hasPrefix("iPhone") } ?? devices.first)?.id
                }
                if devices.isEmpty {
                    simulatorMessage = "Install an iOS runtime in Xcode Settings, then refresh."
                }
            } catch {
                simulatorMessage = error.localizedDescription
            }
        }
    }

    func buildAndRun() {
        guard let project, canRun else { return }
        let trusted = self.trusted
        if isFlutterProject && buildTarget != "ios" {
            let target = buildTarget
            buildArtifact = nil
            perform("Building \(project.suggestedScheme) for \(platformLabel(target))…", work: {
                try $0.buildFlutter(project: project, target: target, trusted: trusted)
            }) { result in
                self.buildArtifact = URL(fileURLWithPath: result.artifact)
                self.status = "The \(platformLabel(target)) build is ready."
            }
            return
        }
        guard let simulator = simulators.first(where: { $0.id == selectedSimulator }) else { return }
        perform("Building \(project.suggestedScheme) and starting \(simulator.name)…", work: {
            try $0.buildAndRun(project: project, simulator: simulator, trusted: trusted, showSimulator: true) { message in
                Task { @MainActor in if self.busy { self.activity = message } }
            }
        }) { result in
            self.status = "\(project.suggestedScheme) is running on \(simulator.name)."
            self.log = result.buildLog
        }
    }

    private func perform<Value: Sendable>(_ message: String, work: @escaping @Sendable (DesktopWorkflow) throws -> Value, done: @escaping (Value) -> Void) {
        guard !busy else { return }
        activity = message
        errorMessage = nil
        log = ""
        Task {
            defer { activity = nil }
            do {
                let result = try await Task.detached(priority: .userInitiated) {
                    try work(DesktopWorkflow.bundled())
                }.value
                done(result)
            } catch {
                errorMessage = error.localizedDescription
                log = (error as? DesktopFailure)?.details ?? ""
            }
        }
    }
}

@MainActor
private struct DesktopView: View {
    @StateObject private var model = DesktopModel()
    @State private var showDetails = false
    @State private var loaded = false
    @State private var showAdmin = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                HStack {
                    HStack(spacing: 10) {
                        DesktopBrand.mark
                        Text("AppRivet").font(.title3.weight(.semibold))
                    }
                    Spacer()
                    Button("Your projects", action: model.openProject).disabled(model.busy)
                    Button("Admin") { showAdmin = true }.disabled(model.busy)
                    Button("Quit") { NSApp.terminate(nil) }
                }
                VStack(alignment: .leading, spacing: 4) {
                    Text("What app do you want to build?").font(.largeTitle.weight(.semibold))
                        .accessibilityAddTraits(.isHeader)
                    Text("Create a SwiftUI project, then run it in the iOS Simulator.")
                        .foregroundStyle(.secondary)
                }
                projectSection.disabled(model.busy)
                simulatorSection.disabled(model.busy)
                statusSection
            }
            .padding(24)
            .frame(maxWidth: 740)
            .frame(maxWidth: .infinity)
        }
        .background { DesktopBrand.backdrop }
        .preferredColorScheme(.dark)
        .frame(minWidth: 540, minHeight: 510)
        .sheet(isPresented: $showAdmin) { AdminSheet(model: model) }
        .sheet(item: $model.draft) { draft in reviewSheet(draft) }
        .onAppear {
            if !loaded {
                loaded = true
                model.refreshSimulators()
            }
        }
    }

    private var projectSection: some View {
        GroupBox("Project") {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Text("Describe your app").font(.headline)
                    Spacer()
                    Button("Import brief", action: model.chooseBrief)
                }
                TextEditor(text: $model.brief)
                    .font(.body)
                    .frame(minHeight: 84)
                    .border(Color.secondary.opacity(0.25))
                    .accessibilityLabel("Describe your app")
                Text(model.briefMessage.isEmpty ? "Describe what the app should do, or import a text or Markdown file." : model.briefMessage)
                    .font(.caption).foregroundStyle(.secondary)
                Picker("Project format", selection: $model.format) {
                    Text("Flutter (shared code)").tag("flutter")
                    Text("SwiftUI (native iOS)").tag("swiftui")
                }
                .onChange(of: model.format) { _ in
                    if model.format == "flutter" && model.starter != "ai" { model.starter = "blank" }
                }
                if model.format == "flutter" {
                    Text("App platforms").font(.headline)
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 120), alignment: .leading)]) {
                        ForEach(appPlatforms.map { $0.id }, id: \.self) { id in
                            Toggle(platformLabel(id), isOn: Binding(
                                get: { model.platforms.contains(id) },
                                set: { checked in
                                    if checked { model.platforms.insert(id) } else { model.platforms.remove(id) }
                                }
                            ))
                            .toggleStyle(.checkbox)
                        }
                    }
                    Link("Flutter SDK and platform setup", destination: URL(string: "https://docs.flutter.dev/install/manual")!)
                        .font(.caption)
                }
                Grid(alignment: .leading, horizontalSpacing: 14, verticalSpacing: 8) {
                    GridRow {
                        Text("Creation method")
                        Picker("Creation method", selection: $model.starter) {
                            Text("Generate with AI").tag("ai")
                            if model.format == "flutter" {
                                Text("Flutter starter").tag("blank")
                            } else {
                                Text("Automatic from brief").tag("auto")
                                Text("Checklist").tag("checklist")
                                Text("Notes").tag("notes")
                                Text("Counter").tag("counter")
                                Text("Blank").tag("blank")
                            }
                        }
                        .labelsHidden()
                    }
                    if model.starter == "ai" {
                        GridRow {
                            Text("Selected model")
                            Text("\(model.selectedProvider.label) · \(model.profiles[model.selectedProvider]?.model ?? "")")
                                .foregroundStyle(.secondary)
                        }
                    }
                    GridRow {
                        Text("App name")
                        TextField("MyFirstApp", text: $model.name).labelsHidden().accessibilityLabel("App name")
                    }
                    if model.format != "flutter" {
                        GridRow {
                            Text("Bundle ID")
                            TextField("Optional", text: $model.bundleID).labelsHidden().accessibilityLabel("Bundle ID")
                        }
                    }
                    GridRow {
                        Text("Save in")
                        HStack {
                            Text(model.parent?.path ?? "Choose a folder")
                                .lineLimit(1).truncationMode(.middle).foregroundStyle(.secondary)
                            Spacer()
                            Button("Choose…", action: model.chooseParent)
                        }
                    }
                }
                HStack {
                    Button(model.project == nil ? "Review prompt" : "Review as new project", action: model.reviewProject)
                        .disabled(!model.canCreate)
                    Button("Open Project…", action: model.openProject)
                }
                if let project = model.project {
                    Divider()
                    HStack(alignment: .firstTextBaseline) {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(verbatim: project.suggestedScheme).font(.headline)
                            Text(verbatim: project.root).font(.caption).foregroundStyle(.secondary)
                                .lineLimit(1).truncationMode(.middle).textSelection(.enabled)
                        }
                        Spacer()
                        Button(project.kind == "flutter-project" ? "Open project folder" : "Open in Xcode") {
                            NSWorkspace.shared.open(URL(fileURLWithPath: project.path, isDirectory: true))
                        }
                    }
                }
            }
            .padding(8)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var simulatorSection: some View {
        GroupBox(model.isFlutterProject ? "Build platform" : "iOS Simulator") {
            VStack(alignment: .leading, spacing: 12) {
                if model.isFlutterProject {
                    if model.localTargets.isEmpty {
                        Text("Open this project on a host with a matching build target.").foregroundStyle(.secondary)
                    } else {
                        Picker("Target", selection: $model.buildTarget) {
                            ForEach(model.localTargets, id: \.self) { target in
                                Text(platformLabel(target)).tag(target)
                            }
                        }
                    }
                }
                if !model.isFlutterProject || model.buildTarget == "ios" {
                    simulatorPicker
                }
                Toggle("I have reviewed this project and trust it to build and run on this Mac", isOn: $model.trusted)
                    .disabled(model.project == nil)
                Button(model.isFlutterProject && model.buildTarget != "ios" ? "Build" : "Build & Run", action: model.buildAndRun)
                    .buttonStyle(.borderedProminent)
                    .disabled(!model.canRun)
                if let artifact = model.buildArtifact {
                    Button("Reveal build in Finder") { NSWorkspace.shared.activateFileViewerSelecting([artifact]) }
                }
            }
            .padding(8)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var simulatorPicker: some View {
        HStack {
            if model.simulators.isEmpty {
                Text(model.simulatorMessage).foregroundStyle(.secondary)
            } else {
                Picker("Device", selection: $model.selectedSimulator) {
                    ForEach(model.simulators) { device in
                        Text(verbatim: device.label).tag(Optional(device.id))
                    }
                }
            }
            Spacer()
            Button("Refresh", action: model.refreshSimulators).disabled(model.loadingSimulators)
        }
    }

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            if let activity = model.activity {
                HStack(spacing: 10) {
                    ProgressView().controlSize(.small)
                    Text(verbatim: activity)
                }
            } else if let error = model.errorMessage {
                Text(verbatim: error).foregroundStyle(.red).textSelection(.enabled)
            } else {
                Text(verbatim: model.status).foregroundStyle(.secondary)
            }
            if !model.log.isEmpty {
                DisclosureGroup("Build details", isExpanded: $showDetails) {
                    ScrollView {
                        Text(verbatim: model.log).font(.system(.caption, design: .monospaced))
                            .textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .frame(maxHeight: 160)
                    .padding(.top, 6)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func reviewSheet(_ draft: DesktopDraft) -> some View {
        let target = draft.provider.flatMap(AIProvider.init(rawValue:))?.label ?? ""
        let ready = !draft.isAI || model.key(for: draft) != nil
        return VStack(alignment: .leading, spacing: 12) {
            Text("Review prompt").font(.title2.bold())
            if let modelID = draft.model {
                Text("Destination: \(target) · \(modelID). Approval sends this prompt and uses your API account.")
                    .foregroundStyle(.secondary)
            } else {
                Text(draft.format == "flutter" ? "Local Flutter starter" : "Local project · \(draft.starter.capitalized) starter")
                    .foregroundStyle(.secondary)
            }
            if let platforms = draft.platforms {
                Text(platforms.map(platformLabel).joined(separator: ", ")).foregroundStyle(.secondary)
            }
            ScrollView {
                Text(verbatim: draft.prompt)
                    .font(.system(.body, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(10)
            }
            .frame(minHeight: 180, maxHeight: 250)
            .border(Color.secondary.opacity(0.25))
            if !ready {
                Text("Add a key for this provider in Admin to send the prompt.")
                    .foregroundStyle(.secondary)
            }
            HStack {
                Button("Back to brief") { model.draft = nil }
                Spacer()
                Button("Copy prompt") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(draft.prompt, forType: .string)
                }
                Button(draft.isAI ? "Send & create project" : (draft.format == "flutter" ? "Create Flutter project" : "Create SwiftUI starter")) {
                    model.createProject(draft)
                }
                    .buttonStyle(.borderedProminent)
                    .disabled(!ready)
            }
        }
        .padding(22)
        .frame(width: 540)
    }
}

@MainActor
private struct AdminSheet: View {
    @ObservedObject var model: DesktopModel
    @Environment(\.dismiss) private var dismiss
    @State private var provider = AIProvider.openai
    @State private var preset = ""
    @State private var custom = ""
    @State private var key = ""
    @State private var feedback = ""

    private var hasKey: Bool { model.profiles[provider]?.key.isEmpty == false }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Admin").font(.title2.bold())
            Text("Choose the AI provider and model. API keys stay in memory for this AppRivet session.")
                .foregroundStyle(.secondary)
            Picker("Provider", selection: $provider) {
                ForEach(AIProvider.allCases) { choice in
                    Text(choice.label).tag(choice)
                }
            }
            Picker("Model", selection: $preset) {
                ForEach(provider.models.map { $0.id }, id: \.self) { id in
                    Text(provider.models.first { $0.id == id }?.label ?? id).tag(id)
                }
                Text("Custom model ID").tag("custom")
            }
            if preset == "custom" {
                TextField("Custom model ID", text: $custom)
            }
            HStack {
                SecureField("Paste API key to add or replace", text: $key)
                    .accessibilityLabel("API key")
                Button("Import key file", action: importKey)
            }
            Text(feedback.isEmpty ? (hasKey ? "Key set for this session." : "No key set for this provider.") : feedback)
                .font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("Remove key") {
                    model.removeKey(for: provider)
                    key = ""
                    feedback = ""
                }
                .disabled(!hasKey)
                Spacer()
                Button("Close") { dismiss() }
                Button("Save settings", action: save)
                    .buttonStyle(.borderedProminent)
            }
        }
        .padding(22)
        .frame(width: 480)
        .onAppear {
            provider = model.selectedProvider
            loadSettings()
        }
        .onChange(of: provider) { _ in loadSettings() }
    }

    private func loadSettings() {
        let selected = model.profiles[provider]?.model ?? provider.models[0].id
        preset = provider.models.contains { $0.id == selected } ? selected : "custom"
        custom = preset == "custom" ? selected : ""
        key = ""
        feedback = ""
    }

    private func importKey() {
        guard let url = chooseTextFile(title: "Import API key", extensions: ["txt"]) else { return }
        do {
            let text = try readTextFile(url, extensions: ["txt"], maxBytes: 1024).trimmingCharacters(in: .whitespacesAndNewlines)
            guard text.range(of: #"^[A-Za-z0-9_-]{8,512}\z"#, options: .regularExpression) != nil else {
                feedback = "Choose a text file containing just one API key."
                return
            }
            key = text
            feedback = "Key loaded. Save settings to use it."
        } catch {
            feedback = error.localizedDescription
        }
    }

    private func save() {
        let modelID = preset == "custom" ? custom.trimmingCharacters(in: .whitespacesAndNewlines) : preset
        let replacement = key.trimmingCharacters(in: .whitespacesAndNewlines)
        guard modelID.range(of: #"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}\z"#, options: .regularExpression) != nil else {
            feedback = "Enter a valid model ID from the provider."
            return
        }
        guard replacement.isEmpty || replacement.range(of: #"^[A-Za-z0-9_-]{8,512}\z"#, options: .regularExpression) != nil else {
            feedback = "Enter a valid API key."
            return
        }
        model.saveSettings(provider: provider, model: modelID, key: replacement)
        dismiss()
    }
}
