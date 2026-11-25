import AppKit
import Darwin
import Security
import Sparkle
import SwiftUI
import UniformTypeIdentifiers

private enum AIProvider: String, CaseIterable, Identifiable, Hashable, Sendable {
    case openai, anthropic

    var id: String { rawValue }
    var label: String { self == .openai ? "OpenAI" : "Anthropic" }
    var modelPreferenceKey: String { "aiModel.\(rawValue)" }
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

private func validModelID(_ value: String) -> Bool {
    value.range(of: #"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}\z"#, options: .regularExpression) != nil
}

private func validAPIKey(_ value: String) -> Bool {
    value.range(of: #"^[A-Za-z0-9_-]{8,512}\z"#, options: .regularExpression) != nil
}

private enum ProviderKeychain {
    private static func attributes(for provider: AIProvider) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: "io.github.fpokrzywa.apprivet.desktop.api-key",
         kSecAttrAccount as String: provider.rawValue]
    }

    static func read(for provider: AIProvider) throws -> String? {
        var query = attributes(for: provider)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        try check(status, action: "read", provider: provider)
        guard let data = result as? Data, let key = String(data: data, encoding: .utf8), validAPIKey(key) else {
            throw DesktopFailure(message: "The saved \(provider.label) key is unreadable. Replace it in Admin.")
        }
        return key
    }

    static func save(_ key: String, for provider: AIProvider) throws {
        let query = attributes(for: provider)
        let data = Data(key.utf8)
        var item = query
        item[kSecValueData as String] = data
        item[kSecAttrLabel as String] = "AppRivet \(provider.label) API key"
        var status = SecItemAdd(item as CFDictionary, nil)
        if status == errSecDuplicateItem {
            status = SecItemUpdate(query as CFDictionary, [kSecValueData as String: data] as CFDictionary)
        }
        try check(status, action: "save", provider: provider)
    }

    static func remove(for provider: AIProvider) throws {
        let status = SecItemDelete(attributes(for: provider) as CFDictionary)
        if status != errSecItemNotFound { try check(status, action: "remove", provider: provider) }
    }

    private static func check(_ status: OSStatus, action: String, provider: AIProvider) throws {
        guard status == errSecSuccess else {
            let reason = SecCopyErrorMessageString(status, nil) as String? ?? "Error \(status)."
            throw DesktopFailure(message: "Could not \(action) the \(provider.label) key in macOS Keychain. \(reason)")
        }
    }
}

private enum PublishingKeychain {
    private static func attributes(for store: DesktopStore) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: "io.github.fpokrzywa.apprivet.desktop.publishing",
         kSecAttrAccount as String: store.rawValue]
    }

    static func contains(_ store: DesktopStore) throws -> Bool {
        var query = attributes(for: store)
        query[kSecReturnAttributes as String] = true
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return false }
        try check(status, action: "find", store: store)
        return true
    }

    static func read(_ store: DesktopStore) throws -> Data {
        var query = attributes(for: store)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        try check(status, action: "read", store: store)
        guard let data = result as? Data else {
            throw DesktopFailure(message: "Replace the unreadable \(store.label) credential in Store Connections.")
        }
        return data
    }

    static func save(_ data: Data, for store: DesktopStore) throws {
        let query = attributes(for: store)
        var item = query
        item[kSecValueData as String] = data
        item[kSecAttrLabel as String] = "AppRivet \(store.label) credential"
        var status = SecItemAdd(item as CFDictionary, nil)
        if status == errSecDuplicateItem {
            status = SecItemUpdate(query as CFDictionary, [kSecValueData as String: data] as CFDictionary)
        }
        try check(status, action: "save", store: store)
    }

    static func remove(_ store: DesktopStore) throws {
        let status = SecItemDelete(attributes(for: store) as CFDictionary)
        if status != errSecItemNotFound { try check(status, action: "remove", store: store) }
    }

    private static func check(_ status: OSStatus, action: String, store: DesktopStore) throws {
        guard status == errSecSuccess else {
            let reason = SecCopyErrorMessageString(status, nil) as String? ?? "Error \(status)."
            throw DesktopFailure(message: "Could not \(action) the \(store.label) credential in macOS Keychain. \(reason)")
        }
    }
}

private let appPlatforms: [(id: String, label: String)] = [
    ("ios", "iOS"), ("android", "Android"), ("web", "Web"),
    ("windows", "Windows"), ("macos", "macOS"), ("linux", "Linux"),
]

private func platformLabel(_ id: String) -> String {
    appPlatforms.first { $0.id == id }?.label ?? id
}

private enum WorkspaceAppearance: String, Hashable {
    case dark, light

    var scheme: ColorScheme { self == .dark ? .dark : .light }
}

@MainActor
private enum DesktopBrand {
    private static let lightMark = Bundle.main.url(forResource: "AppRivetMarkLight", withExtension: "png")
        .flatMap { NSImage(contentsOf: $0) }
    private static let darkMark = Bundle.main.url(forResource: "AppRivetMarkDark", withExtension: "png")
        .flatMap { NSImage(contentsOf: $0) }

    @ViewBuilder static func mark(_ appearance: WorkspaceAppearance) -> some View {
        if let artwork = appearance == .dark ? lightMark : darkMark {
            Image(nsImage: artwork)
                .resizable()
                .scaledToFit()
                .frame(width: 46, height: 46)
                .accessibilityHidden(true)
        }
    }

    static func backdrop(_ appearance: WorkspaceAppearance) -> some View {
        let dark = appearance == .dark
        return GeometryReader { geometry in
            ZStack {
                dark ? Color(red: 0.063, green: 0.094, blue: 0.122) : Color(red: 0.929, green: 0.957, blue: 0.949)
                RadialGradient(colors: [dark ? Color(red: 0.114, green: 0.278, blue: 0.282) : Color(red: 0.761, green: 0.867, blue: 0.839), .clear],
                               center: UnitPoint(x: 0.08, y: 0), startRadius: 0, endRadius: geometry.size.width * 0.65)
                RadialGradient(colors: [dark ? Color(red: 0.149, green: 0.224, blue: 0.29) : Color(red: 0.859, green: 0.906, blue: 0.941), .clear],
                               center: UnitPoint(x: 0.88, y: 0.30), startRadius: 0, endRadius: geometry.size.width * 0.54)
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }
}

@MainActor
private func chooseTextFile(title: String, extensions: [String], message: String = "") -> URL? {
    let panel = NSOpenPanel()
    panel.title = title
    panel.message = message
    panel.prompt = "Import"
    panel.canChooseDirectories = false
    panel.allowedContentTypes = extensions.compactMap { UTType(filenameExtension: $0) }
    guard panel.runModal() == .OK else { return nil }
    return panel.url
}

private func readFile(_ url: URL, extensions: [String], maxBytes: Int? = nil) throws -> Data {
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
    return bytes
}

private func readTextFile(_ url: URL, extensions: [String], maxBytes: Int? = nil) throws -> String {
    let bytes = try readFile(url, extensions: extensions, maxBytes: maxBytes)
    guard var text = String(data: bytes, encoding: .utf8) else {
        throw DesktopFailure(message: "Choose a file saved as UTF-8 plain text.")
    }
    if text.hasPrefix("\u{feff}") { text.removeFirst() }
    return text.replacingOccurrences(of: "\r\n", with: "\n").replacingOccurrences(of: "\r", with: "\n")
}

private func importStoreCredential(_ file: URL, store: DesktopStore) throws {
    let maxBytes = (store == .apple ? 8 : 24) * 1024
    let data = try readFile(file, extensions: [store.credentialExtension], maxBytes: maxBytes)
    let text = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines)
    switch store {
    case .apple:
        guard let text, text.hasPrefix("-----BEGIN PRIVATE KEY-----"), text.hasSuffix("-----END PRIVATE KEY-----") else {
            throw DesktopFailure(message: "Choose an App Store Connect API private key in a .p8 file.")
        }
    case .google:
        let account = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
        guard text != nil, account?["type"] as? String == "service_account",
              let email = account?["client_email"] as? String, !email.isEmpty,
              let privateKey = account?["private_key"] as? String, !privateKey.isEmpty else {
            throw DesktopFailure(message: "Choose a Google service account key in a .json file.")
        }
    }
    try PublishingKeychain.save(data, for: store)
}

private func importBrief(_ url: URL) throws -> String {
    let text = try readTextFile(url, extensions: ["txt", "md", "markdown"])
    guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
          !text.unicodeScalars.contains(where: { $0.properties.generalCategory == .control && $0.value != 9 && $0.value != 10 }) else {
        throw DesktopFailure(message: "Choose a file containing plain text or Markdown.")
    }
    return text
}

@MainActor
private final class UpdateCheckState: ObservableObject {
    @Published var canCheck = false

    init(_ updater: SPUUpdater) {
        updater.publisher(for: \.canCheckForUpdates).assign(to: &$canCheck)
    }
}

@main
struct AppRivetDesktop: App {
    private let updaterController: SPUStandardUpdaterController
    @StateObject private var updateCheck: UpdateCheckState

    init() {
        let arguments = CommandLine.arguments
        let smokeTest = arguments.dropFirst().first == "--smoke-test"
        let controller = SPUStandardUpdaterController(startingUpdater: !smokeTest, updaterDelegate: nil, userDriverDelegate: nil)
        updaterController = controller
        _updateCheck = StateObject(wrappedValue: UpdateCheckState(controller.updater))
        guard smokeTest else { return }
        do {
            guard arguments.count == 3 else { throw DesktopFailure(message: "Smoke test requires a parent folder.") }
            try controller.updater.start()
            guard Bundle.main.url(forResource: "AppRivetMarkLight", withExtension: "png") != nil,
                  Bundle.main.url(forResource: "AppRivetMarkDark", withExtension: "png") != nil else {
                throw DesktopFailure(message: "The AppRivet emblems are missing from the package.")
            }
            let workflow = try DesktopWorkflow.bundled()
            guard try workflow.projectFormats() == ["swiftui"] else {
                throw DesktopFailure(message: "The desktop exposed a format for a missing toolchain.")
            }
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
            let planRequest = DesktopPlanRequest(name: "DesktopPlan", bundleID: "", brief: String(imported.prefix(13 * 1024)),
                                                 format: "swiftui", platforms: ["ios"], provider: "anthropic", model: "claude-opus-5-5")
            let planReview = try workflow.reviewPlan(planRequest)
            guard planReview.usesSynthesis == true, planReview.approval.count == 64,
                  planReview.prompts.count > 1, planReview.requestCount == planReview.prompts.count + 1,
                  planReview.maxOutputTokens > planReview.prompts.reduce(0, { $0 + $1.maxOutputTokens }),
                  planReview.prompts.allSatisfy({ !$0.prompt.isEmpty }) else {
                throw DesktopFailure(message: "The local planning review did not include the expected extraction prompts and synthesis allowance.")
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
            Self.smokeProgress("Checking the store publishing workspace…")
            _ = try workflow.publishingSettings()
            guard try workflow.storeBuildHistory(project: project).isEmpty else {
                throw DesktopFailure(message: "A new project unexpectedly has a release build history.")
            }
            Self.smokeProgress("Checking Flutter project reopening and Web preview…")
            try Self.smokeWebPreviewAndReopen(workflow, parent: parent)
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

    private static func smokeWebPreviewAndReopen(_ workflow: DesktopWorkflow, parent: URL) throws {
        let root = parent.appendingPathComponent("Renamed Flutter Project", isDirectory: true)
        let selection = root.appendingPathComponent("ios/Runner.xcodeproj", isDirectory: true)
        for directory in [selection, root.appendingPathComponent("lib"), root.appendingPathComponent("web"),
                          root.appendingPathComponent("build/web")] {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        }
        let page = "<!doctype html><title>AppRivet Web preview smoke test</title>"
        let files = [
            "pubspec.yaml": "name: apprivet_previewfixture\n",
            "lib/main.dart": "void main() {}\n",
            "ios/Runner.xcodeproj/project.pbxproj": "",
            "APP_BRIEF.md": "Assemble a watch and show the time.\n",
            ".apprivet.json": #"{"version":1,"settings":{"format":"flutter","name":"PreviewFixture","starter":"ai","provider":"anthropic","model":"claude-opus-5-5"}}"#,
            "build/web/index.html": page,
        ]
        for (name, body) in files {
            try Data(body.utf8).write(to: root.appendingPathComponent(name))
        }
        let opened = try workflow.open(selection)
        let model = DesktopModel()
        model.restoreProject(opened)
        guard model.name == "PreviewFixture", model.brief == "Assemble a watch and show the time.",
              model.format == "flutter", model.platforms == ["ios", "web"], model.starter == "ai",
              model.selectedProvider == .anthropic, model.profiles[.anthropic]?.model == "claude-opus-5-5" else {
            throw DesktopFailure(message: "The Flutter project selection did not restore the saved form.")
        }
        let preview = try workflow.previewWeb(project: opened.project, trusted: true)
        guard try String(contentsOf: preview.url, encoding: .utf8) == page else {
            throw DesktopFailure(message: "The Web preview did not serve the completed build.")
        }
    }

    var body: some Scene {
        Window("AppRivet", id: "main") {
            DesktopView(updateCheck: updateCheck, checkForUpdates: updaterController.updater.checkForUpdates)
        }
        .defaultSize(width: 680, height: 740)
        .commands {
            CommandGroup(after: .appInfo) {
                Button("Check for Updates…", action: updaterController.updater.checkForUpdates)
                    .disabled(!updateCheck.canCheck)
            }
        }
    }
}

private enum CompletedBuild {
    case simulator
    case flutter(DesktopFlutterBuild)
}

private struct DesktopPlanSession: Identifiable, Sendable {
    let id = UUID()
    let request: DesktopPlanRequest
    let parent: URL
    var review: DesktopPlanReview
}

@MainActor
private final class DesktopModel: ObservableObject {
    @Published var name = "MyFirstApp"
    @Published var brief = ""
    @Published var starter = "blank"
    @Published var stageShortBriefs = false
    @Published private(set) var format = ""
    @Published private(set) var projectFormats = ["swiftui"]
    @Published private(set) var checkingFormats = false
    @Published var platforms: Set<String> = ["ios", "android", "web"]
    @Published var buildTarget = "web"
    @Published var appearance = WorkspaceAppearance.dark
    @Published var bundleID = ""
    @Published var parent: URL?
    @Published var selectedSimulator: String?
    @Published var trusted = false
    @Published var draft: DesktopDraft?
    @Published var planSession: DesktopPlanSession?
    @Published private(set) var selectedProvider: AIProvider
    @Published private(set) var briefMessage = ""
    @Published private(set) var profiles: [AIProvider: AIProfile]
    @Published private(set) var loadingProviderKeys = false
    @Published private(set) var keychainErrors: [AIProvider: String] = [:]
    @Published private(set) var project: DesktopProject?
    @Published private(set) var projectSelectionID = UUID()
    @Published private(set) var simulators: [DesktopSimulator] = []
    @Published private(set) var loadingSimulators = false
    @Published private(set) var simulatorMessage = "No iOS simulators available"
    @Published private(set) var activity: String?
    @Published private(set) var status = "Choose a folder to create your first app."
    @Published private(set) var errorMessage: String?
    @Published private(set) var log = ""
    @Published private(set) var completedBuild: CompletedBuild?
    @Published private(set) var storeHistory: [DesktopStoreBuild] = []
    @Published private(set) var loadingStoreHistory = false
    @Published private(set) var storeHistoryError: String?
    private var webPreview: DesktopWebPreview?
    private let preferences: UserDefaults

    init(preferences: UserDefaults = .standard) {
        self.preferences = preferences
        selectedProvider = preferences.string(forKey: "aiProvider").flatMap(AIProvider.init(rawValue:)) ?? .openai
        profiles = Dictionary(uniqueKeysWithValues: AIProvider.allCases.map { provider in
            let saved = preferences.string(forKey: provider.modelPreferenceKey) ?? ""
            return (provider, AIProfile(model: validModelID(saved) ? saved : provider.models[0].id))
        })
    }

    var busy: Bool { activity != nil }
    var flutterAvailable: Bool { projectFormats.contains("flutter") }
    var hasSelectedFormat: Bool { projectFormats.contains(format) }
    var largeBrief: Bool { brief.trimmingCharacters(in: .whitespacesAndNewlines).utf8.count > 4096 }
    var usesStagedGeneration: Bool { starter == "ai" && (largeBrief || stageShortBriefs) }
    var canCreate: Bool {
        guard hasSelectedFormat, let parent else { return false }
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
        guard project != nil, trusted, !isFlutterProject || flutterAvailable else { return false }
        return isFlutterProject && buildTarget != "ios" ? localTargets.contains(buildTarget) : selectedSimulator != nil
    }
    var storeTargets: [String] {
        guard let project else { return [] }
        return project.kind == "xcode-project" ? ["ios"] : localTargets.filter { $0 == "ios" || $0 == "android" }
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

    func loadProviderKeys() {
        guard !loadingProviderKeys else { return }
        loadingProviderKeys = true
        Task {
            let results = await Task.detached(priority: .userInitiated) {
                AIProvider.allCases.map { provider -> (AIProvider, String, String?) in
                    do { return (provider, try ProviderKeychain.read(for: provider) ?? "", nil) }
                    catch { return (provider, "", error.localizedDescription) }
                }
            }.value
            for (provider, key, error) in results {
                profiles[provider]?.key = key
                keychainErrors[provider] = error
            }
            loadingProviderKeys = false
        }
    }

    func saveSettings(provider: AIProvider, model: String, key: String) async throws {
        if !key.isEmpty {
            try await Task.detached(priority: .userInitiated) { try ProviderKeychain.save(key, for: provider) }.value
            keychainErrors[provider] = nil
        }
        profiles[provider] = AIProfile(model: model, key: key.isEmpty ? profiles[provider]?.key ?? "" : key)
        selectedProvider = provider
        preferences.set(provider.rawValue, forKey: "aiProvider")
        preferences.set(model, forKey: provider.modelPreferenceKey)
    }

    func removeKey(for provider: AIProvider) async throws {
        try await Task.detached(priority: .userInitiated) { try ProviderKeychain.remove(for: provider) }.value
        profiles[provider]?.key = ""
        keychainErrors[provider] = nil
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
        panel.message = "Choose the AppRivet project folder, its saved brief, or its Xcode project."
        panel.canChooseDirectories = true
        panel.canChooseFiles = true
        guard panel.runModal() == .OK, let url = panel.url else { return }
        perform("Opening project…", work: { try $0.open(url) }, done: restoreProject)
    }

    func restoreProject(_ opened: DesktopOpened) {
        let settings = opened.settings
        name = settings.name
        brief = settings.brief ?? ""
        briefMessage = ""
        bundleID = settings.bundleID ?? ""
        format = settings.format
        platforms = Set(settings.platforms)
        starter = settings.starter
        stageShortBriefs = false
        if let provider = settings.provider.flatMap(AIProvider.init(rawValue:)),
           let model = settings.model, validModelID(model) {
            selectedProvider = provider
            profiles[provider]?.model = model
        }
        parent = URL(fileURLWithPath: opened.project.root, isDirectory: true).deletingLastPathComponent()
        selectProject(opened.project)
        status = "Opened \(settings.name). Use a different project name or save folder to create a new copy."
    }

    func reviewProject() {
        guard canCreate, let parent else { return }
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let format = self.format
        let bundleID = format == "flutter" ? "" : bundleID.trimmingCharacters(in: .whitespacesAndNewlines)
        let brief = self.brief
        let starter = self.starter
        let provider = selectedProvider
        let model = profiles[provider]?.model ?? ""
        let platforms = format == "flutter" ? appPlatforms.map { $0.id }.filter { self.platforms.contains($0) } : ["ios"]
        if usesStagedGeneration {
            let request = DesktopPlanRequest(name: name, bundleID: bundleID, brief: brief, format: format,
                                             platforms: platforms, provider: provider.rawValue, model: model)
            perform("Preparing planning prompts locally…", work: { try $0.reviewPlan(request) }) {
                self.planSession = DesktopPlanSession(request: request, parent: parent, review: $0)
            }
            return
        }
        perform("Validating the app brief…", work: {
            try $0.review(name: name, bundleID: bundleID, brief: brief, starter: starter, provider: provider.rawValue, model: model,
                          format: format, platforms: platforms)
        }) { self.draft = $0 }
    }

    func key(for providerID: String?) -> String? {
        guard let provider = providerID.flatMap(AIProvider.init(rawValue:)),
              let key = profiles[provider]?.key, !key.isEmpty else { return nil }
        return key
    }

    func missingKeyMessage(for providerID: String?) -> String {
        if loadingProviderKeys { return "Checking macOS Keychain…" }
        if let provider = providerID.flatMap(AIProvider.init(rawValue:)), let error = keychainErrors[provider] {
            return "\(error) Check this provider’s key in Admin."
        }
        return "Add and save a key for this provider in Admin."
    }

    func createProject(_ draft: DesktopDraft) {
        guard projectFormats.contains(draft.format ?? "swiftui"), let parent else { return }
        let key = key(for: draft.provider)
        guard !draft.isAI || key != nil else { return }
        self.draft = nil
        perform(draft.isAI ? "Waiting for the model to generate the source…" : "Creating \(draft.name)…", work: {
            try $0.create(draft: draft, parent: parent, apiKey: key)
        }, done: selectProject)
    }

    func sendPlanningRequests(_ session: DesktopPlanSession) {
        guard session.review.plan == nil, let key = key(for: session.request.provider) else { return }
        perform("Waiting for the model to prepare a component plan…", work: {
            try $0.draftPlan(session.request, review: session.review, apiKey: key)
        }) { self.planSession?.review = $0 }
    }

    func createProject(_ session: DesktopPlanSession) {
        guard session.review.plan != nil, projectFormats.contains(session.request.format),
              let key = key(for: session.request.provider) else { return }
        perform("Waiting for the model to generate the approved source files…", work: {
            try $0.createFromPlan(session.request, review: session.review, parent: session.parent, apiKey: key)
        }) { project in
            self.planSession = nil
            self.selectProject(project)
        }
    }

    func chooseFormat(_ format: String) {
        guard projectFormats.contains(format) else { return }
        self.format = format
        if format == "flutter" && starter != "ai" { starter = "blank" }
    }

    func refreshProjectFormats() {
        guard !checkingFormats else { return }
        checkingFormats = true
        Task {
            defer { checkingFormats = false }
            do {
                projectFormats = try await Task.detached(priority: .userInitiated) {
                    try DesktopWorkflow.bundled().projectFormats()
                }.value
            } catch {
                self.errorMessage = error.localizedDescription
            }
            if format.isEmpty { chooseFormat(projectFormats.first ?? "swiftui") }
        }
    }

    private func selectProject(_ project: DesktopProject) {
        self.project = project
        projectSelectionID = UUID()
        trusted = false
        completedBuild = nil
        storeHistory = []
        loadingStoreHistory = false
        storeHistoryError = nil
        webPreview = nil
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
        guard let project, canRun, !busy else { return }
        let trusted = self.trusted
        completedBuild = nil
        if isFlutterProject && buildTarget != "ios" {
            let target = buildTarget
            if target == "web" { webPreview = nil }
            perform("Building \(project.suggestedScheme) for \(platformLabel(target))…", work: {
                try $0.buildFlutter(project: project, target: target, trusted: trusted)
            }) { result in
                self.completedBuild = .flutter(result)
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
            self.completedBuild = .simulator
        }
    }

    func loadStoreHistory() {
        guard let project, !loadingStoreHistory else { return }
        let selection = projectSelectionID
        loadingStoreHistory = true
        storeHistoryError = nil
        Task {
            defer { if projectSelectionID == selection { loadingStoreHistory = false } }
            do {
                let history = try await Task.detached(priority: .userInitiated) {
                    try DesktopWorkflow.bundled().storeBuildHistory(project: project)
                }.value
                if projectSelectionID == selection { storeHistory = history }
            } catch {
                if projectSelectionID == selection { storeHistoryError = error.localizedDescription }
            }
        }
    }

    func buildForStore(_ platform: String) {
        guard let project, trusted, !busy, !loadingStoreHistory, storeTargets.contains(platform),
              !isFlutterProject || flutterAvailable else { return }
        let selection = projectSelectionID
        let trusted = self.trusted
        perform("Building \(project.suggestedScheme) for the \(platform == "ios" ? "App Store" : "Play Store")…", work: {
            try $0.buildForStore(project: project, platform: platform, trusted: trusted)
        }) { result in
            guard self.projectSelectionID == selection else { return }
            self.storeHistory.removeAll { $0.id == result.id }
            self.storeHistory.insert(result, at: 0)
            self.status = "The \(platformLabel(result.platform)) store build is ready in Finder."
        }
    }

    func openWebBuild() {
        guard let project, trusted, !busy else { return }
        if let webPreview, webPreview.isRunning {
            openPreviewInBrowser(webPreview.url)
            return
        }
        perform("Starting the Web preview…", work: { try $0.previewWeb(project: project, trusted: true) }) { preview in
            self.webPreview = preview
            self.openPreviewInBrowser(preview.url)
        }
    }

    private func openPreviewInBrowser(_ url: URL) {
        if NSWorkspace.shared.open(url) {
            errorMessage = nil
            status = "Web build opened in your browser."
        } else {
            errorMessage = "Could not open the Web build in your default browser."
        }
    }

    func showSimulator() {
        perform("Opening Simulator…", work: { try $0.showSimulator() }) { _ in }
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
    private enum ProjectTab: String, Hashable { case generate, publish }

    @ObservedObject var updateCheck: UpdateCheckState
    let checkForUpdates: () -> Void
    @StateObject private var model = DesktopModel()
    @State private var showDetails = false
    @State private var loaded = false
    @State private var showAdmin = false
    @State private var projectTab = ProjectTab.generate

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                HStack {
                    HStack(spacing: 10) {
                        DesktopBrand.mark(model.appearance)
                        Text("AppRivet").font(.title3.weight(.semibold))
                    }
                    Spacer()
                    Button("Your projects", action: model.openProject).disabled(model.busy)
                    Button("Admin") { showAdmin = true }.disabled(model.busy)
                    Button("Check for Updates…", action: checkForUpdates).disabled(!updateCheck.canCheck || model.busy)
                    Button("Quit") { NSApp.terminate(nil) }
                }
                if model.project != nil {
                    Picker("Project view", selection: $projectTab) {
                        Text("Generate & Build").tag(ProjectTab.generate)
                        Text("Publish").tag(ProjectTab.publish)
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    .disabled(model.busy)
                }
                if let project = model.project, projectTab == .publish {
                    StorePublishPane(model: model, project: project)
                        .id(model.projectSelectionID)
                        .disabled(model.busy)
                } else {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("What app do you want to build?").font(.largeTitle.weight(.semibold))
                            .accessibilityAddTraits(.isHeader)
                        Text(model.flutterAvailable
                             ? "Create a SwiftUI or Flutter project, then build it for your selected platform."
                             : "Create a SwiftUI project, then run it in the iOS Simulator.")
                            .foregroundStyle(.secondary)
                    }
                    projectSection.disabled(model.busy)
                    simulatorSection.disabled(model.busy)
                }
                statusSection
            }
            .padding(24)
            .frame(maxWidth: 740)
            .frame(maxWidth: .infinity)
        }
        .background { DesktopBrand.backdrop(model.appearance) }
        .preferredColorScheme(model.appearance.scheme)
        .frame(minWidth: 540, minHeight: 510)
        .sheet(isPresented: $showAdmin) { AdminSheet(model: model) }
        .sheet(item: $model.draft) { draft in reviewSheet(draft) }
        .sheet(item: $model.planSession) { _ in DesktopPlanSheet(model: model) }
        .onAppear {
            if !loaded {
                loaded = true
                model.loadProviderKeys()
                model.refreshProjectFormats()
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
                Picker("Project format", selection: Binding(
                    get: { model.hasSelectedFormat ? model.format : "" },
                    set: model.chooseFormat
                )) {
                    if !model.hasSelectedFormat { Text("Choose a project format").tag("") }
                    if model.flutterAvailable { Text("Flutter (shared code)").tag("flutter") }
                    Text("SwiftUI (native iOS)").tag("swiftui")
                }
                if !model.flutterAvailable {
                    HStack {
                        if model.checkingFormats {
                            Text("Checking installed SDKs…")
                        } else {
                            Link("Install the Flutter SDK to enable Flutter projects", destination: URL(string: "https://docs.flutter.dev/install/manual")!)
                            Button("Check again", action: model.refreshProjectFormats)
                        }
                    }
                    .font(.caption)
                }
                if model.hasSelectedFormat && model.format == "flutter" {
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
                    if model.hasSelectedFormat {
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
                    }
                    GridRow {
                        Text("App name")
                        TextField("MyFirstApp", text: $model.name).labelsHidden().accessibilityLabel("App name")
                    }
                    if model.format == "swiftui" {
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
                if model.hasSelectedFormat && model.starter == "ai" {
                    if model.largeBrief {
                        Text("This brief will use component planning before code generation.")
                            .font(.caption).foregroundStyle(.secondary)
                    } else {
                        Toggle("Plan components before generating code", isOn: $model.stageShortBriefs)
                            .toggleStyle(.checkbox)
                    }
                }
                HStack {
                    Button(model.usesStagedGeneration ? "Review planning requests" : (model.project == nil ? "Review prompt" : "Review as new project"),
                           action: model.reviewProject)
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
                if model.isFlutterProject && !model.flutterAvailable {
                    Text("Install Flutter to build this project.").foregroundStyle(.secondary)
                } else {
                    buildControls
                }
            }
            .padding(8)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    @ViewBuilder
    private var buildControls: some View {
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
        if let build = model.completedBuild {
            buildActions(build)
        }
    }

    @ViewBuilder
    private func buildActions(_ build: CompletedBuild) -> some View {
        switch build {
        case .simulator:
            Button("Show Simulator", action: model.showSimulator)
        case .flutter(let build):
            let artifact = URL(fileURLWithPath: build.artifact)
            if build.target == "web" {
                Button("Open in Browser", action: model.openWebBuild).disabled(!model.trusted)
            }
            if build.target == "android" {
                Button("Show APK in Finder") { NSWorkspace.shared.activateFileViewerSelecting([artifact]) }
                Text("Drag the APK onto a running Android Studio emulator to install it.")
                    .font(.callout).foregroundStyle(.secondary)
            } else {
                Button("Reveal build in Finder") { NSWorkspace.shared.activateFileViewerSelecting([artifact]) }
            }
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
        let ready = !draft.isAI || model.key(for: draft.provider) != nil
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
                Text(model.missingKeyMessage(for: draft.provider))
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
private struct StorePublishPane: View {
    @ObservedObject var model: DesktopModel
    let project: DesktopProject
    @State private var target = ""

    private var selectedTarget: String { model.storeTargets.contains(target) ? target : model.storeTargets.first ?? "" }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                Text(verbatim: project.suggestedScheme).font(.largeTitle.weight(.semibold))
                    .accessibilityAddTraits(.isHeader)
                Text(verbatim: project.root).font(.caption).foregroundStyle(.secondary)
                    .lineLimit(1).truncationMode(.middle).textSelection(.enabled)
            }
            GroupBox("Build for Store") {
                VStack(alignment: .leading, spacing: 10) {
                    buildControls
                }
                .padding(8)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            GroupBox("Store build history") {
                VStack(alignment: .leading, spacing: 10) {
                    if model.loadingStoreHistory {
                        ProgressView("Loading store builds…").controlSize(.small)
                    } else if let error = model.storeHistoryError {
                        Text(verbatim: error).foregroundStyle(.secondary)
                        Button("Reload history", action: model.loadStoreHistory)
                    } else if model.storeHistory.isEmpty {
                        Text("No store builds for this project yet.").foregroundStyle(.secondary)
                    }
                    ForEach(model.storeHistory) { build in
                        if build.id != model.storeHistory.first?.id { Divider() }
                        buildRow(build)
                    }
                }
                .padding(8)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .onAppear(perform: model.loadStoreHistory)
    }

    @ViewBuilder
    private var buildControls: some View {
        if model.storeTargets.isEmpty {
            Text("This project has no iOS or Android target.").foregroundStyle(.secondary)
        } else if model.isFlutterProject && !model.flutterAvailable {
            HStack {
                Text("Install Flutter to build this project for a store.").foregroundStyle(.secondary)
                Button("Check again", action: model.refreshProjectFormats).disabled(model.checkingFormats)
            }
        } else {
            Picker("Target", selection: Binding(get: { selectedTarget }, set: { target = $0 })) {
                ForEach(model.storeTargets, id: \.self) { platform in
                    Text(platform == "ios" ? "iOS (.ipa)" : "Android (.aab)").tag(platform)
                }
            }
            Text(selectedTarget == "ios"
                 ? "Requires Xcode, an Apple Distribution signing certificate, and an App Store provisioning profile for this app. Save the Apple Team ID in Store Connections."
                 : "Requires the Android SDK and Flutter release signing with your Google Play upload key. Save the upload certificate SHA-256 fingerprint in Store Connections.")
                .font(.callout).foregroundStyle(.secondary)
            Text("Open Admin to find Store Connections. Use Generate & Build for simulator or emulator testing. Upload the completed store build manually in the store console.")
                .font(.callout).foregroundStyle(.secondary)
            Toggle("I have reviewed this project and trust it to build and run on this Mac", isOn: $model.trusted)
            Button("Build for Store") { model.buildForStore(selectedTarget) }
                .buttonStyle(.borderedProminent)
                .disabled(!model.trusted || model.loadingStoreHistory)
        }
    }

    private func buildRow(_ build: DesktopStoreBuild) -> some View {
        let file = URL(fileURLWithPath: build.artifact)
        return VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(verbatim: file.lastPathComponent).font(.headline)
                    .lineLimit(1).truncationMode(.middle)
                Spacer()
                Button("Show in Finder") { NSWorkspace.shared.activateFileViewerSelecting([file]) }
            }
            Text("\(platformLabel(build.platform)) · \(ByteCountFormatter.string(fromByteCount: build.size, countStyle: .file))")
                .font(.caption).foregroundStyle(.secondary)
            Text("SHA-256 \(build.sha256)").font(.caption2.monospaced()).foregroundStyle(.secondary)
                .textSelection(.enabled)
        }
    }
}

@MainActor
private struct DesktopPlanSheet: View {
    @ObservedObject var model: DesktopModel

    var body: some View {
        if let session = model.planSession {
            reviewSheet(session)
                .interactiveDismissDisabled(model.busy)
        }
    }

    private func reviewSheet(_ session: DesktopPlanSession) -> some View {
        let review = session.review
        let ready = model.key(for: session.request.provider) != nil
        let target = AIProvider(rawValue: session.request.provider)?.label ?? session.request.provider
        let generating = review.plan != nil
        return VStack(alignment: .leading, spacing: 12) {
            Text(generating ? "Review component code requests" : "Review planning requests")
                .font(.title2.bold())
            Text("\(target) \(session.request.model) will receive these requests through your API account.")
                .foregroundStyle(.secondary)
            Text("\(review.requestCount) \(review.requestCount == 1 ? "request" : "requests"), up to \(review.maxOutputTokens) output tokens total.")
            if !generating {
                Text("After planning, you can review the exact code requests before approving them together.")
                    .foregroundStyle(.secondary)
            }
            if review.usesSynthesis == true {
                let synthesisCount = review.requestCount - review.prompts.count
                let synthesisTokens = review.maxOutputTokens - review.prompts.reduce(0) { $0 + $1.maxOutputTokens }
                let requests = synthesisCount == 1 ? "1 follow-up request" : "\(synthesisCount) follow-up requests"
                Text("The plan uses \(requests) to combine the extracted requirements. Exact input depends on the extraction results. Up to \(synthesisTokens) output tokens for these requests are included in the total.")
                    .foregroundStyle(.secondary)
            }
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 16) {
                    if let plan = review.plan {
                        Text(verbatim: plan.architecture).textSelection(.enabled)
                    }
                    ForEach(Array(review.prompts.enumerated()), id: \.offset) { _, prompt in
                        VStack(alignment: .leading, spacing: 6) {
                            Text(verbatim: prompt.title).font(.headline)
                            if let file = prompt.file {
                                Text(verbatim: file).font(.system(.subheadline, design: .monospaced))
                                    .textSelection(.enabled)
                            }
                            Text("\(prompt.prompt.utf8.count) UTF-8 input bytes, up to \(prompt.maxOutputTokens) output tokens")
                                .font(.caption).foregroundStyle(.secondary)
                            Text(verbatim: prompt.prompt)
                                .font(.system(.body, design: .monospaced))
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                }
                .padding(10)
            }
            .frame(minHeight: 220, maxHeight: 360)
            .border(Color.secondary.opacity(0.25))
            if let activity = model.activity {
                ProgressView(activity)
            }
            if let error = model.errorMessage {
                Text(verbatim: error).foregroundStyle(.red).textSelection(.enabled)
            }
            if !ready {
                Text(model.missingKeyMessage(for: session.request.provider))
                    .foregroundStyle(.secondary)
            }
            HStack {
                Button("Back to brief") { model.planSession = nil }
                    .disabled(model.busy)
                Spacer()
                Button(generating ? "Approve all code requests & create" : "Send to plan") {
                    if generating { model.createProject(session) } else { model.sendPlanningRequests(session) }
                }
                .buttonStyle(.borderedProminent)
                .disabled(model.busy || !ready)
            }
        }
        .padding(22)
        .frame(width: 640)
    }
}

@MainActor
private struct AdminSheet: View {
    private enum Tab: Hashable { case general, stores }

    @ObservedObject var model: DesktopModel
    @Environment(\.dismiss) private var dismiss
    @State private var provider = AIProvider.openai
    @State private var appearance = WorkspaceAppearance.dark
    @State private var preset = ""
    @State private var custom = ""
    @State private var key = ""
    @State private var feedback = ""
    @State private var updatingSettings = false
    @State private var tab = Tab.general

    private var hasKey: Bool { model.profiles[provider]?.key.isEmpty == false }
    private var keyStatus: String {
        if !feedback.isEmpty { return feedback }
        if model.loadingProviderKeys { return "Checking macOS Keychain…" }
        if let error = model.keychainErrors[provider] { return error }
        return hasKey ? "Key saved in macOS Keychain." : "No key saved for this provider."
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Settings").font(.title2.bold())
            Picker("Settings section", selection: $tab) {
                Text("General").tag(Tab.general)
                Text("Store Connections").tag(Tab.stores)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            if tab == .general {
                generalSettings
            } else {
                StoreConnectionsView(updatingSettings: $updatingSettings, close: { dismiss() })
            }
        }
        .padding(22)
        .frame(width: 500)
        .disabled(updatingSettings)
        .interactiveDismissDisabled(updatingSettings)
        .onAppear {
            provider = model.selectedProvider
            appearance = model.appearance
            loadSettings()
            if !model.keychainErrors.isEmpty { model.loadProviderKeys() }
        }
        .onChange(of: provider) { _ in loadSettings() }
    }

    private var generalSettings: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Choose the AI provider and model. Save settings stores your API key in macOS Keychain.")
                .foregroundStyle(.secondary)
            Picker("Appearance", selection: $appearance) {
                Text("Dark").tag(WorkspaceAppearance.dark)
                Text("Light").tag(WorkspaceAppearance.light)
            }
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
                Button("Import key (.txt)", action: importKey)
            }
            Text("Import a UTF-8 .txt file containing only one API key for the selected provider.")
                .font(.caption).foregroundStyle(.secondary)
            Text(keyStatus)
                .font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("Remove key") {
                    updateSettings {
                        try await model.removeKey(for: provider)
                        key = ""
                        feedback = "Key removed from macOS Keychain."
                    }
                }
                .disabled(model.loadingProviderKeys || (!hasKey && model.keychainErrors[provider] == nil))
                Spacer()
                Button("Close") { dismiss() }
                Button("Save settings", action: save)
                    .buttonStyle(.borderedProminent)
                    .disabled(model.loadingProviderKeys)
            }
        }
    }

    private func loadSettings() {
        let selected = model.profiles[provider]?.model ?? provider.models[0].id
        preset = provider.models.contains { $0.id == selected } ? selected : "custom"
        custom = preset == "custom" ? selected : ""
        key = ""
        feedback = ""
    }

    private func importKey() {
        guard let url = chooseTextFile(title: "Import API key", extensions: ["txt"],
                                       message: "Select a UTF-8 .txt file containing only one \(provider.label) API key.") else { return }
        do {
            let text = try readTextFile(url, extensions: ["txt"], maxBytes: 1024).trimmingCharacters(in: .whitespacesAndNewlines)
            guard validAPIKey(text) else {
                feedback = "Choose a UTF-8 .txt file containing only one API key."
                return
            }
            key = text
            feedback = "Key loaded. Save settings to keep it in macOS Keychain."
        } catch {
            feedback = error.localizedDescription
        }
    }

    private func save() {
        let modelID = preset == "custom" ? custom.trimmingCharacters(in: .whitespacesAndNewlines) : preset
        let replacement = key.trimmingCharacters(in: .whitespacesAndNewlines)
        guard validModelID(modelID) else {
            feedback = "Enter a valid model ID from the provider."
            return
        }
        guard replacement.isEmpty || validAPIKey(replacement) else {
            feedback = "Enter a valid API key."
            return
        }
        updateSettings {
            try await model.saveSettings(provider: provider, model: modelID, key: replacement)
            model.appearance = appearance
            key = ""
            dismiss()
        }
    }

    private func updateSettings(_ action: @escaping @MainActor () async throws -> Void) {
        guard !updatingSettings && !model.loadingProviderKeys else { return }
        updatingSettings = true
        feedback = "Updating settings…"
        Task {
            defer { updatingSettings = false }
            do { try await action() }
            catch { feedback = error.localizedDescription }
        }
    }
}

@MainActor
private struct StoreConnectionsView: View {
    @Binding var updatingSettings: Bool
    let close: () -> Void
    @State private var settings = DesktopPublishingSettings()
    @State private var savedSettings: DesktopPublishingSettings?
    @State private var credentialPresent: [DesktopStore: Bool] = [:]
    @State private var storeMessages: [DesktopStore: String] = [:]
    @State private var feedback = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if savedSettings != nil {
                ScrollView {
                    VStack(alignment: .leading, spacing: 12) {
                        GroupBox(DesktopStore.apple.label) {
                            VStack(alignment: .leading, spacing: 8) {
                                metadataField("Apple Team ID", text: $settings.appleTeamID)
                                metadataField("Issuer ID (team API keys)", text: $settings.appleIssuerID)
                                metadataField("Key ID", text: $settings.appleKeyID)
                                credentialControls(.apple)
                                Text("Save the Key ID and the Issuer ID for team keys before testing. The connection check reads App Store Connect data.")
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            .padding(6)
                        }
                        GroupBox(DesktopStore.google.label) {
                            VStack(alignment: .leading, spacing: 8) {
                                metadataField("Upload certificate SHA-256", text: $settings.androidUploadCertificateSHA256)
                                credentialControls(.google)
                                Text("Use the upload key certificate fingerprint from Play Console’s App integrity page.")
                                    .font(.caption).foregroundStyle(.secondary)
                                Text("Test Connection verifies that Google issues an access token for the service account. App permissions are managed in Play Console.")
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            .padding(6)
                        }
                    }
                }
                .frame(maxHeight: 370)
            }
            if !feedback.isEmpty {
                Text(verbatim: feedback).font(.caption).foregroundStyle(.secondary)
            }
            HStack {
                if updatingSettings { ProgressView().controlSize(.small) }
                if savedSettings == nil && !updatingSettings {
                    Button("Retry", action: loadSettings)
                }
                Spacer()
                Button("Close", action: close)
                if let savedSettings {
                    Button("Save settings", action: saveSettings)
                        .buttonStyle(.borderedProminent)
                        .disabled(settings == savedSettings)
                }
            }
        }
        .onAppear(perform: loadSettings)
    }

    private func metadataField(_ label: String, text: Binding<String>) -> some View {
        LabeledContent(label) {
            TextField("", text: text).accessibilityLabel(label)
        }
    }

    private func credentialControls(_ store: DesktopStore) -> some View {
        let fileType = store.credentialExtension
        let present = credentialPresent[store]
        return VStack(alignment: .leading, spacing: 6) {
            HStack {
                Button(present == true ? "Replace (.\(fileType))…" : "Import (.\(fileType))…") { importCredential(store) }
                Button("Remove") { removeCredential(store) }.disabled(present == false)
                Spacer()
                Button("Test Connection") { testConnection(store) }
                    .disabled(present != true || (store == .apple && !appleSettingsReady))
            }
            if let present {
                Text(present ? "Credential saved in macOS Keychain." : "No credential saved.")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if let message = storeMessages[store] {
                Text(verbatim: message).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            }
        }
    }

    private var appleSettingsReady: Bool {
        guard let savedSettings else { return false }
        return !savedSettings.appleKeyID.isEmpty && settings.appleKeyID == savedSettings.appleKeyID
            && settings.appleIssuerID == savedSettings.appleIssuerID
    }

    private func loadSettings() {
        update("Loading store settings…") {
            let saved = try await Task.detached(priority: .userInitiated) { try DesktopWorkflow.bundled().publishingSettings() }.value
            settings = saved
            savedSettings = saved
            let credentials = await Task.detached(priority: .userInitiated) {
                DesktopStore.allCases.map { store -> (DesktopStore, Bool?, String?) in
                    do { return (store, try PublishingKeychain.contains(store), nil) }
                    catch { return (store, nil, error.localizedDescription) }
                }
            }.value
            for (store, present, error) in credentials {
                credentialPresent[store] = present
                storeMessages[store] = error
            }
            feedback = ""
        }
    }

    private func saveSettings() {
        let replacement = settings
        update("Saving store settings…") {
            let saved = try await Task.detached(priority: .userInitiated) {
                try DesktopWorkflow.bundled().savePublishingSettings(replacement)
            }.value
            if saved.appleIssuerID != savedSettings?.appleIssuerID || saved.appleKeyID != savedSettings?.appleKeyID {
                storeMessages[.apple] = nil
            }
            settings = saved
            savedSettings = saved
            feedback = "Store settings saved."
        }
    }

    private func importCredential(_ store: DesktopStore) {
        guard let file = chooseTextFile(title: "Import \(store.label) credential", extensions: [store.credentialExtension]) else { return }
        update("Saving credential in macOS Keychain…") {
            try await Task.detached(priority: .userInitiated) { try importStoreCredential(file, store: store) }.value
            credentialPresent[store] = true
            storeMessages[store] = nil
            feedback = ""
        }
    }

    private func removeCredential(_ store: DesktopStore) {
        update("Removing credential from macOS Keychain…") {
            try await Task.detached(priority: .userInitiated) { try PublishingKeychain.remove(store) }.value
            credentialPresent[store] = false
            storeMessages[store] = nil
            feedback = ""
        }
    }

    private func testConnection(_ store: DesktopStore) {
        storeMessages[store] = nil
        update("Testing \(store.label) connection…") {
            let message = try await Task.detached(priority: .userInitiated) {
                let credential = try PublishingKeychain.read(store)
                return try DesktopWorkflow.bundled().checkStoreConnection(store, credential: credential)
            }.value
            storeMessages[store] = message
            feedback = ""
        }
    }

    private func update(_ progress: String, action: @escaping @MainActor () async throws -> Void) {
        guard !updatingSettings else { return }
        updatingSettings = true
        feedback = progress
        Task {
            defer { updatingSettings = false }
            do { try await action() }
            catch { feedback = error.localizedDescription }
        }
    }
}
