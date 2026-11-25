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

    enum CodingKeys: String, CodingKey {
        case kind, path, root, starter
        case suggestedScheme = "suggested_scheme"
    }
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

    func create(name: String, bundleID: String, parent: URL, brief: String, starter: String) throws -> DesktopProject {
        guard name.range(of: #"^[A-Za-z][A-Za-z0-9]{0,49}\z"#, options: .regularExpression) != nil else {
            throw DesktopFailure(message: "Start the project name with a letter and use up to 50 letters and numbers.")
        }
        return try invoke(["init", "--bundle-id=\(bundleID)", "--brief=\(brief)", "--starter=\(starter)", name], directory: parent)
    }

    func inspect(_ url: URL) throws -> DesktopProject {
        let project: DesktopProject = try invoke(["inspect", "--project", url.path])
        guard project.kind == "xcode-project" else {
            throw DesktopFailure(message: "Choose a generated AppRivet project folder or its .xcodeproj file.")
        }
        return project
    }

    func simulators() throws -> [DesktopSimulator] {
        let inventory: SimulatorInventory = try invoke(["devices", "--simulators", "--timeout", "5m"], timeout: 310)
        return inventory.devices.filter { $0.platform == "iOS" && $0.available == true && UUID(uuidString: $0.id) != nil }
            .sorted { $0.label.localizedStandardCompare($1.label) == .orderedAscending }
    }

    func buildAndRun(project: DesktopProject, simulator: DesktopSimulator, trusted: Bool, showSimulator: Bool,
                     progress: @Sendable (String) -> Void = { _ in }) throws -> DesktopRun {
        guard trusted else { throw DesktopFailure(message: "Confirm you trust this project before building it.") }
        guard simulator.platform == "iOS", simulator.available == true, UUID(uuidString: simulator.id) != nil else {
            throw DesktopFailure(message: "Select an available iOS simulator.")
        }
        let name = project.suggestedScheme
        guard project.kind == "xcode-project",
              name.range(of: #"^[A-Za-z][A-Za-z0-9]{0,49}\z"#, options: .regularExpression) != nil else {
            throw DesktopFailure(message: "Open an AppRivet project with its generated app name and shared scheme.")
        }
        progress("Building \(name)…")
        let build: DesktopBuild = try invoke([
            "build", "--project", project.path, "--scheme", name, "--simulator", simulator.id,
            "--trust-project", "--timeout", "15m",
        ], timeout: 920)
        let (app, bundleID) = try simulatorProduct(build: build, name: name)
        do {
            progress("Starting \(simulator.name)…")
            _ = try appleTool("/usr/bin/xcrun", ["simctl", "bootstatus", simulator.id, "-b"], timeout: 480)
            progress("Installing \(name)…")
            _ = try appleTool("/usr/bin/xcrun", ["simctl", "install", simulator.id, app.path], timeout: 240)
            progress("Launching \(name)…")
            _ = try appleTool("/usr/bin/xcrun", ["simctl", "launch", "--terminate-running-process", simulator.id, bundleID], timeout: 240)
            if showSimulator {
                let developer = try appleTool("/usr/bin/xcode-select", ["-p"], timeout: 15)
                    .trimmingCharacters(in: .whitespacesAndNewlines)
                let simulatorApp = URL(fileURLWithPath: developer, isDirectory: true)
                    .appendingPathComponent("Applications/Simulator.app", isDirectory: true)
                _ = try appleTool("/usr/bin/open", ["-a", simulatorApp.path], timeout: 30)
            }
        } catch let failure as DesktopFailure {
            throw DesktopFailure(message: "The build succeeded. \(failure.message)", details: failure.details + "\n" + build.execution.output)
        }
        return DesktopRun(bundleIdentifier: bundleID, buildLog: build.execution.output)
    }

    private func simulatorProduct(build: DesktopBuild, name: String) throws -> (URL, String) {
        let root = URL(fileURLWithPath: build.derivedData, isDirectory: true).resolvingSymlinksInPath()
        let app = root.appendingPathComponent("Build/Products/Debug-iphonesimulator/\(name).app", isDirectory: true)
        let info = app.appendingPathComponent("Info.plist")
        guard build.derivedData.hasPrefix("/"),
              app.resolvingSymlinksInPath().path == app.path,
              info.resolvingSymlinksInPath().path == info.path,
              let plistData = try? Data(contentsOf: info),
              let plist = (try? PropertyListSerialization.propertyList(from: plistData, options: [], format: nil)) as? [String: Any],
              let platforms = plist["CFBundleSupportedPlatforms"] as? [String], platforms.contains("iPhoneSimulator"),
              let bundleID = plist["CFBundleIdentifier"] as? String,
              bundleID.range(of: #"^[A-Za-z0-9][A-Za-z0-9.-]*\z"#, options: .regularExpression) != nil else {
            throw DesktopFailure(message: "The build did not produce the expected iOS Simulator app \(name).app.", details: build.execution.output)
        }
        return (app, bundleID)
    }

    private func invoke<Value: Decodable>(_ args: [String], directory: URL? = nil, timeout: Int = 30) throws -> Value {
        let result = try runProcess(cli, args + ["--json"], directory: directory, timeout: timeout)
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
private func runProcess(_ executable: URL, _ args: [String], directory: URL? = nil, timeout: Int) throws -> ProcessResult {
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

    let child = Process()
    child.executableURL = executable
    child.arguments = args
    child.currentDirectoryURL = directory ?? manager.temporaryDirectory
    child.standardInput = FileHandle.nullDevice
    child.standardOutput = output
    child.standardError = errors
    child.environment = ProcessInfo.processInfo.environment.filter { key, _ in
        key != "DEVELOPER_DIR" && !key.hasPrefix("SIMCTL_CHILD_")
    }

    func readOutput() throws -> ProcessResult {
        try output.seek(toOffset: 0)
        try errors.seek(toOffset: 0)
        return ProcessResult(status: child.isRunning ? -1 : child.terminationStatus,
                             stdout: try output.read(upToCount: 4 * 1024 * 1024) ?? Data(),
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
