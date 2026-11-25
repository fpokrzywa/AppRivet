import AppKit
import Darwin
import SwiftUI

@main
struct AppRivetDesktop: App {
    init() {
        let arguments = CommandLine.arguments
        guard arguments.dropFirst().first == "--smoke-test" else { return }
        do {
            guard arguments.count == 3 else { throw DesktopFailure(message: "Smoke test requires a parent folder.") }
            let workflow = try DesktopWorkflow.bundled()
            let parent = URL(fileURLWithPath: arguments[2], isDirectory: true)
            Self.smokeProgress("Creating the SwiftUI test project…")
            let created = try workflow.create(name: "DesktopSmoke", bundleID: "org.example.apprivetdesktop", parent: parent,
                                              brief: "A checklist for a weekend trip.", starter: "auto")
            guard created.starter == "checklist" else { throw DesktopFailure(message: "The app brief did not select the checklist starter.") }
            let project = try workflow.inspect(URL(fileURLWithPath: created.root, isDirectory: true))
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
    @Published var starter = "auto"
    @Published var bundleID = ""
    @Published var parent: URL?
    @Published var selectedSimulator: String?
    @Published var trusted = false
    @Published private(set) var project: DesktopProject?
    @Published private(set) var simulators: [DesktopSimulator] = []
    @Published private(set) var activity: String?
    @Published private(set) var status = "Choose a folder to create your first app."
    @Published private(set) var errorMessage: String?
    @Published private(set) var log = ""

    var busy: Bool { activity != nil }
    var canCreate: Bool { parent != nil && !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
    var canRun: Bool { project != nil && selectedSimulator != nil && trusted }

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
        perform("Opening project…", work: { try $0.inspect(url) }, done: selectProject)
    }

    func createProject() {
        guard let parent else { return }
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let bundleID = bundleID.trimmingCharacters(in: .whitespacesAndNewlines)
        let brief = self.brief
        let starter = self.starter
        perform("Creating \(name)…", work: { try $0.create(name: name, bundleID: bundleID, parent: parent, brief: brief, starter: starter) }, done: selectProject)
    }

    private func selectProject(_ project: DesktopProject) {
        self.project = project
        trusted = false
        if let starter = project.starter {
            status = "\(project.suggestedScheme) was created with the \(starter) starter."
        } else {
            status = "\(project.suggestedScheme) is ready to build."
        }
    }

    func refreshSimulators() {
        perform("Looking for simulators…", work: { try $0.simulators() }) { devices in
            self.simulators = devices
            if !devices.contains(where: { $0.id == self.selectedSimulator }) {
                self.selectedSimulator = (devices.first { $0.name.hasPrefix("iPhone") } ?? devices.first)?.id
            }
            if devices.isEmpty {
                self.status = "Install an iOS Simulator runtime in Xcode Settings, then refresh."
            } else if let project = self.project {
                self.status = "\(project.suggestedScheme) is ready to build."
            } else {
                self.status = "Create or open an AppRivet project to get started."
            }
        }
    }

    func buildAndRun() {
        guard let project, let simulator = simulators.first(where: { $0.id == selectedSimulator }), trusted else { return }
        let trusted = self.trusted
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

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("What app do you want to build?").font(.largeTitle.weight(.semibold))
                        .accessibilityAddTraits(.isHeader)
                    Text("Create a SwiftUI starter, then run it in the iOS Simulator.")
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
        .frame(minWidth: 540, minHeight: 510)
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
                Text("Describe your app").font(.headline)
                TextEditor(text: $model.brief)
                    .font(.body)
                    .frame(minHeight: 84)
                    .border(Color.secondary.opacity(0.25))
                    .accessibilityLabel("Describe your app")
                Text("For example: A checklist for packing on weekend trips.")
                    .font(.caption).foregroundStyle(.secondary)
                Grid(alignment: .leading, horizontalSpacing: 14, verticalSpacing: 8) {
                    GridRow {
                        Text("Starter")
                        Picker("Starter", selection: $model.starter) {
                            Text("Automatic from brief").tag("auto")
                            Text("Checklist").tag("checklist")
                            Text("Notes").tag("notes")
                            Text("Counter").tag("counter")
                            Text("Blank").tag("blank")
                        }
                        .labelsHidden()
                    }
                    GridRow {
                        Text("App name")
                        TextField("MyFirstApp", text: $model.name).labelsHidden().accessibilityLabel("App name")
                    }
                    GridRow {
                        Text("Bundle ID")
                        TextField("Optional", text: $model.bundleID).labelsHidden().accessibilityLabel("Bundle ID")
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
                    Button("Create SwiftUI Starter", action: model.createProject).disabled(!model.canCreate)
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
                        Button("Open in Xcode") {
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
        GroupBox("iOS Simulator") {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    if model.simulators.isEmpty {
                        Text("No iOS simulators available").foregroundStyle(.secondary)
                    } else {
                        Picker("Device", selection: $model.selectedSimulator) {
                            ForEach(model.simulators) { device in
                                Text(verbatim: device.label).tag(Optional(device.id))
                            }
                        }
                    }
                    Spacer()
                    Button("Refresh", action: model.refreshSimulators)
                }
                Toggle("I trust this project to run build scripts on this Mac", isOn: $model.trusted)
                    .disabled(model.project == nil)
                Button("Build & Run", action: model.buildAndRun)
                    .buttonStyle(.borderedProminent)
                    .disabled(!model.canRun)
            }
            .padding(8)
            .frame(maxWidth: .infinity, alignment: .leading)
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
}
