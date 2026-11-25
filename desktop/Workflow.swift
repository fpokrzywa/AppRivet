import Foundation
import Darwin

struct DesktopFailure: LocalizedError {
    let message: String
    var details = ""
    var errorDescription: String? { message }
}

struct DesktopProject: Decodable, Sendable {
    let kind: String
    let path: String
    let root: String
    let suggestedScheme: String
    let starter: String?
    let platforms: [String]?

    enum CodingKeys: String, CodingKey {
        case kind, path, root, starter, platforms
        case suggestedScheme = "suggested_scheme"
    }
}

struct DesktopDraft: Decodable, Identifiable, Sendable {
    let name: String
    let bundleID: String
    let starter: String
    let starterChoice: String
    let brief: String
    let prompt: String
    let provider: String?
    let model: String?
    let approval: String?
    let format: String?
    let platforms: [String]?

    var id: String { approval ?? name }
    var isAI: Bool { provider != nil }

    enum CodingKeys: String, CodingKey {
        case name, starter, brief, prompt, provider, model, approval, format, platforms
        case bundleID = "bundle_id"
        case starterChoice = "starter_choice"
    }
}

struct DesktopOpened: Decodable, Sendable {
    struct Settings: Decodable, Sendable {
        let name: String
        let format: String
        let platforms: [String]
        let starter: String
        let bundleID: String?
        let provider: String?
        let model: String?
        let brief: String?

        enum CodingKeys: String, CodingKey {
            case name, format, platforms, starter, provider, model, brief
            case bundleID = "bundle_id"
        }
    }

    let project: DesktopProject
    let settings: Settings
}

struct DesktopSimulator: Decodable, Identifiable, Sendable {
    let id: String
    let name: String
    let platform: String
    let runtime: String?
    let available: Bool?

    var label: String {
        let prefix = "com.apple.CoreSimulator.SimRuntime.iOS-"
        guard let runtime, runtime.hasPrefix(prefix) else { return name }
        let version = runtime.dropFirst(prefix.count).replacingOccurrences(of: "-", with: ".")
        return "\(name) (iOS \(version))"
    }
}

private struct SimulatorInventory: Decodable {
    let devices: [DesktopSimulator]
}

private struct DesktopBuild: Decodable {
    struct Execution: Decodable { let output: String }
    let derivedData: String
    let execution: Execution

    enum CodingKeys: String, CodingKey {
        case derivedData = "derived_data"
        case execution
    }
}

private struct CLIEnvelope<Value: Decodable>: Decodable {
    struct Failure: Decodable { let message: String }
    let schema_version: Int
    let ok: Bool
    let data: Value?
    let error: Failure?
}

struct DesktopRun: Sendable {
    let bundleIdentifier: String
    let buildLog: String
}

struct DesktopFlutterBuild: Decodable, Sendable {
    let target: String
    let artifact: String
}

final class DesktopWebPreview: @unchecked Sendable {
    let url: URL
    private let child: Process
    private let lifetime: FileHandle

    init(url: URL, child: Process, lifetime: FileHandle) {
        self.url = url
        self.child = child
        self.lifetime = lifetime
    }

    var isRunning: Bool { child.isRunning }

    deinit { try? lifetime.close() }
}

struct DesktopWorkflow: Sendable {
    let cli: URL

    static func bundled() throws -> Self {
        guard let executable = Bundle.main.executableURL else {
            throw DesktopFailure(message: "Open AppRivet from its app bundle in Finder.")
        }
        let cli = executable.deletingLastPathComponent().appendingPathComponent("apprivet")
        guard FileManager.default.isExecutableFile(atPath: cli.path) else {
            throw DesktopFailure(message: "The AppRivet command-line tool is missing from this copy of the app.")
        }
        return Self(cli: cli)
    }

    func review(name: String, bundleID: String, brief: String, starter: String, provider: String, model: String,
                format: String = "swiftui", platforms: [String] = []) throws -> DesktopDraft {
        var args = ["init", "--review", "--bundle-id=\(bundleID)", "--brief-stdin", "--starter=\(starter)"]
        args += ["--format=\(format)", "--platforms=\(platforms.joined(separator: ","))"]
        if starter == "ai" {
            args += ["--provider=\(provider)", "--model=\(model)"]
        }
        return try invoke(args + [name], input: Data(brief.utf8), fullOutput: true)
    }

    func create(draft: DesktopDraft, parent: URL, apiKey: String? = nil) throws -> DesktopProject {
        var args = ["init", "--bundle-id=\(draft.bundleID)", "--brief-stdin"]
        if draft.format == "flutter", let platforms = draft.platforms {
            args += ["--format=flutter", "--platforms=\(platforms.joined(separator: ","))"]
        }
        if let provider = draft.provider, let model = draft.model, let approval = draft.approval {
            args += ["--provider=\(provider)", "--model=\(model)", "--approve=\(approval)"]
        } else {
            args.append("--starter=\(draft.starterChoice)")
        }
        // Allow the provider request and Flutter scaffolding to run sequentially.
        let timeout = draft.isAI ? (draft.format == "flutter" ? 690 : 390) : (draft.format == "flutter" ? 540 : 30)
        return try invoke(args + [draft.name], directory: parent, timeout: timeout,
                          apiKey: apiKey, input: Data(draft.brief.utf8))
    }

    func inspect(_ url: URL) throws -> DesktopProject {
        let project: DesktopProject = try invoke(["inspect", "--project", url.path])
        guard project.kind == "xcode-project" || project.kind == "flutter-project" else {
            throw DesktopFailure(message: "Choose a generated Flutter project folder or a SwiftUI project.")
        }
        return project
    }

    func open(_ url: URL) throws -> DesktopOpened {
        try invoke(["inspect", "--form", "--project", url.path], fullOutput: true)
    }

    func projectFormats() throws -> [String] {
        try invoke(["doctor", "--project-formats"])
    }

    func simulators() throws -> [DesktopSimulator] {
        let inventory: SimulatorInventory = try invoke(["devices", "--simulators", "--timeout", "5m"], timeout: 310)
        return inventory.devices.filter { $0.platform == "iOS" && $0.available == true && UUID(uuidString: $0.id) != nil }
            .sorted { $0.label.localizedStandardCompare($1.label) == .orderedAscending }
    }

    func buildFlutter(project: DesktopProject, target: String, trusted: Bool) throws -> DesktopFlutterBuild {
        guard trusted else { throw DesktopFailure(message: "Confirm you trust this project before building it.") }
        return try invoke(["flutter-build", "--project", project.root, "--target", target, "--trust-project"], timeout: 930)
    }

    func previewWeb(project: DesktopProject, trusted: Bool) throws -> DesktopWebPreview {
        guard trusted else { throw DesktopFailure(message: "Confirm you trust this project before opening its web build.") }
        let child = Process()
        let input = Pipe()
        let output = Pipe()
        child.executableURL = cli
        child.arguments = ["flutter-preview", "--project", project.root, "--trust-project"]
        child.currentDirectoryURL = FileManager.default.temporaryDirectory
        child.standardInput = input
        child.standardOutput = output
        child.standardError = FileHandle.nullDevice
        child.environment = [:]
        try child.run()
        let lifetime = input.fileHandleForWriting
        let ready = output.fileHandleForReading
        var descriptor = pollfd(fd: ready.fileDescriptor, events: Int16(POLLIN), revents: 0)
        // Read the single readiness line without waiting for EOF on the live child process.
        var bytes = [UInt8](repeating: 0, count: 256)
        let received = poll(&descriptor, 1, 8000) > 0 ? bytes.withUnsafeMutableBytes {
            Darwin.read(descriptor.fd, $0.baseAddress, $0.count)
        } : -1
        guard received > 0,
              let url = URL(string: String(decoding: bytes.prefix(received), as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)),
              let port = url.port, (1...65535).contains(port),
              url.absoluteString == "http://127.0.0.1:\(port)/" else {
            try? lifetime.close()
            if child.isRunning { child.terminate() }
            throw DesktopFailure(message: "Could not start the Web preview. Rebuild the Web target and try again.")
        }
        return DesktopWebPreview(url: url, child: child, lifetime: lifetime)
    }

    func showSimulator() throws {
        let developer = try appleTool("/usr/bin/xcode-select", ["-p"], timeout: 15)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let simulatorApp = URL(fileURLWithPath: developer, isDirectory: true)
            .appendingPathComponent("Applications/Simulator.app", isDirectory: true)
        _ = try appleTool("/usr/bin/open", ["-a", simulatorApp.path], timeout: 30)
    }

    func buildAndRun(project: DesktopProject, simulator: DesktopSimulator, trusted: Bool, showSimulator: Bool,
                     progress: @Sendable (String) -> Void = { _ in }) throws -> DesktopRun {
        guard trusted else { throw DesktopFailure(message: "Confirm you trust this project before building it.") }
        guard simulator.platform == "iOS", simulator.available == true, UUID(uuidString: simulator.id) != nil else {
            throw DesktopFailure(message: "Select an available iOS simulator.")
        }
        let name = project.suggestedScheme
        progress("Building \(name)…")
        let app: URL
        let log: String
        if project.kind == "flutter-project" {
            let product = try buildFlutter(project: project, target: "ios", trusted: trusted)
            app = URL(fileURLWithPath: product.artifact, isDirectory: true)
            log = ""
        } else {
            guard project.kind == "xcode-project",
                  name.range(of: #"^[A-Za-z][A-Za-z0-9]{0,49}\z"#, options: .regularExpression) != nil else {
                throw DesktopFailure(message: "Open an AppRivet project with its generated app name and shared scheme.")
            }
            let build: DesktopBuild = try invoke([
                "build", "--project", project.path, "--scheme", name, "--simulator", simulator.id,
                "--trust-project", "--timeout", "15m",
            ], timeout: 920)
            guard build.derivedData.hasPrefix("/") else {
                throw DesktopFailure(message: "The build returned an invalid product directory.")
            }
            let root = URL(fileURLWithPath: build.derivedData, isDirectory: true).resolvingSymlinksInPath()
            app = root.appendingPathComponent("Build/Products/Debug-iphonesimulator/\(name).app", isDirectory: true)
            log = build.execution.output
        }
        let bundleID = try simulatorBundleID(app, details: log)
        do {
            progress("Starting \(simulator.name)…")
            _ = try appleTool("/usr/bin/xcrun", ["simctl", "bootstatus", simulator.id, "-b"], timeout: 480)
            progress("Installing \(name)…")
            _ = try appleTool("/usr/bin/xcrun", ["simctl", "install", simulator.id, app.path], timeout: 240)
            progress("Launching \(name)…")
            _ = try appleTool("/usr/bin/xcrun", ["simctl", "launch", "--terminate-running-process", simulator.id, bundleID], timeout: 240)
            if showSimulator { try self.showSimulator() }
        } catch let failure as DesktopFailure {
            throw DesktopFailure(message: "The build succeeded. \(failure.message)", details: failure.details + "\n" + log)
        }
        return DesktopRun(bundleIdentifier: bundleID, buildLog: log)
    }

    private func simulatorBundleID(_ app: URL, details: String) throws -> String {
        let info = app.appendingPathComponent("Info.plist")
        guard app.resolvingSymlinksInPath().path == app.path,
              info.resolvingSymlinksInPath().path == info.path,
              let plistData = try? Data(contentsOf: info),
              let plist = (try? PropertyListSerialization.propertyList(from: plistData, options: [], format: nil)) as? [String: Any],
              let platforms = plist["CFBundleSupportedPlatforms"] as? [String], platforms.contains("iPhoneSimulator"),
              let bundleID = plist["CFBundleIdentifier"] as? String,
              bundleID.range(of: #"^[A-Za-z0-9][A-Za-z0-9.-]*\z"#, options: .regularExpression) != nil else {
            throw DesktopFailure(message: "The build did not produce the expected iOS Simulator app.", details: details)
        }
        return bundleID
    }

    private func invoke<Value: Decodable>(_ args: [String], directory: URL? = nil, timeout: Int = 30,
                                          apiKey: String? = nil, input: Data? = nil, fullOutput: Bool = false) throws -> Value {
        let result = try runProcess(cli, args + ["--json"], directory: directory, timeout: timeout,
                                    apiKey: apiKey, input: input, fullOutput: fullOutput)
        guard let response = try? JSONDecoder().decode(CLIEnvelope<Value>.self, from: result.stdout), response.schema_version == 1 else {
            throw DesktopFailure(message: "AppRivet returned an unreadable result.", details: result.diagnostics)
        }
        guard result.status == 0, response.ok, let value = response.data else {
            let buildLog = (response.data as? DesktopBuild)?.execution.output ?? result.diagnostics
            throw DesktopFailure(message: response.error?.message ?? "AppRivet did not complete this action.", details: buildLog)
        }
        return value
    }

    private func appleTool(_ path: String, _ args: [String], timeout: Int) throws -> String {
        let result = try runProcess(URL(fileURLWithPath: path), args, timeout: timeout)
        guard result.status == 0 else {
            let action = args.first == "simctl" ? (args.dropFirst().first ?? "command") : URL(fileURLWithPath: path).lastPathComponent
            throw DesktopFailure(message: "Xcode’s Simulator \(action) command failed.", details: result.diagnostics)
        }
        return String(decoding: result.stdout, as: UTF8.self)
    }
}

private struct ProcessResult {
    let status: Int32
    let stdout: Data
    let stderr: Data

    var diagnostics: String {
        [stderr, stdout].map { String(decoding: $0.prefix(256 * 1024), as: UTF8.self) }
            .filter { !$0.isEmpty }.joined(separator: "\n")
    }
}

// Use files for separate stdout and stderr so a verbose child cannot deadlock
// on a full pipe. Bound what the app reads back into memory.
private func runProcess(_ executable: URL, _ args: [String], directory: URL? = nil, timeout: Int,
                        apiKey: String? = nil, input: Data? = nil, fullOutput: Bool = false) throws -> ProcessResult {
    let manager = FileManager.default
    let scratch = manager.temporaryDirectory.appendingPathComponent("apprivet-ui-\(UUID().uuidString)", isDirectory: true)
    try manager.createDirectory(at: scratch, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
    defer { try? manager.removeItem(at: scratch) }
    let stdout = scratch.appendingPathComponent("stdout")
    let stderr = scratch.appendingPathComponent("stderr")
    guard manager.createFile(atPath: stdout.path, contents: nil), manager.createFile(atPath: stderr.path, contents: nil) else {
        throw DesktopFailure(message: "Could not prepare temporary command output files.")
    }
    let output = try FileHandle(forUpdating: stdout)
    defer { try? output.close() }
    let errors = try FileHandle(forUpdating: stderr)
    defer { try? errors.close() }
    let standardInput: FileHandle?
    if let input {
        let stdin = scratch.appendingPathComponent("stdin")
        try input.write(to: stdin, options: .withoutOverwriting)
        standardInput = try FileHandle(forReadingFrom: stdin)
    } else {
        standardInput = nil
    }
    defer { try? standardInput?.close() }

    let child = Process()
    child.executableURL = executable
    child.arguments = args
    child.currentDirectoryURL = directory ?? manager.temporaryDirectory
    child.standardInput = standardInput ?? FileHandle.nullDevice
    child.standardOutput = output
    child.standardError = errors
    var environment = ProcessInfo.processInfo.environment.filter { key, _ in
        key != "DEVELOPER_DIR" && key != "APPRIVET_API_KEY" && !key.hasPrefix("SIMCTL_CHILD_")
    }
    environment["APPRIVET_API_KEY"] = apiKey
    child.environment = environment

    func readOutput() throws -> ProcessResult {
        try output.seek(toOffset: 0)
        try errors.seek(toOffset: 0)
        let stdout = try fullOutput ? output.readToEnd() : output.read(upToCount: 4 * 1024 * 1024)
        return ProcessResult(status: child.isRunning ? -1 : child.terminationStatus,
                             stdout: stdout ?? Data(),
                             stderr: try errors.read(upToCount: 256 * 1024) ?? Data())
    }

    let exited = DispatchSemaphore(value: 0)
    child.terminationHandler = { _ in exited.signal() }
    try child.run()
    if exited.wait(timeout: .now() + .seconds(timeout)) == .timedOut {
        if child.isRunning { child.terminate() }
        if exited.wait(timeout: .now() + .seconds(5)) == .timedOut, child.isRunning {
            kill(child.processIdentifier, SIGKILL)
        }
        let command = args.first == "simctl" ? "simctl \(args.dropFirst().first ?? "")" : executable.lastPathComponent
        throw DesktopFailure(message: "The \(command) command timed out. Check Xcode and Simulator, then try again.",
                             details: (try? readOutput().diagnostics) ?? "")
    }
    return try readOutput()
}
