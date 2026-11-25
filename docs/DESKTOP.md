# AppRivet for macOS

Open **AppRivet.app** from Finder to create, build, and run a SwiftUI app in an iOS Simulator. The app includes the AppRivet command-line tool. You need full Xcode and an iOS Simulator runtime on your Mac. The simulator flow uses unsigned builds and works without a phone or developer signing team.

## Get the app

1. Open a successful [AppRivet CI run](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml) and download **AppRivet-desktop-download** under **Artifacts**. Sign in to GitHub if prompted.
2. Extract the download, then extract **AppRivet-macOS.zip** inside it. Move **AppRivet.app** to Applications or another folder you use for apps. The archive includes Apple silicon and Intel executables.
3. Install full Xcode and open it once to finish setup. In **Xcode → Settings → Components**, install an iOS runtime compatible with the generated app’s iOS 17 minimum. Select the full Xcode installation under **Settings → Locations → Command Line Tools**. Exact runtime availability depends on your Xcode release. [Xcode system requirements](https://developer.apple.com/xcode/system-requirements/) · [Installing components](https://developer.apple.com/documentation/xcode/downloading-and-installing-additional-xcode-components)
4. Open **AppRivet.app**. The CI download uses an ad hoc signature and has no Apple notarization. If macOS blocks it as an unverified developer and you trust the download, macOS provides an **Open Anyway** option under **System Settings → Privacy & Security** after you try to open it. [Apple’s Gatekeeper guidance](https://support.apple.com/en-us/102445)

## Create and run an app

1. Enter an app name such as `MyFirstApp`. Leave **Bundle ID** blank to use the default. Pick a save folder, then click **Create App**. AppRivet creates a new subfolder and refuses to overwrite an existing project. Use **Open Project…** to reopen a previous project.
2. Choose an available iPhone or iPad simulator. If the list is empty after first setup, check the runtime in Xcode and click **Refresh** in AppRivet.
3. Confirm that you trust the project to run build scripts, then click **Build & Run**. AppRivet builds the shared scheme matching the generated project name, boots the selected simulator if needed, installs the iOS Simulator product and launches the app. The first boot can take a few minutes.
4. Use **Open in Xcode** to edit the app. If build or launch fails, read the status message and expand **Build details** for command output.

This first version supports generated SwiftUI Xcode projects. Build scripts run with the access of your macOS user account. Build output is kept in the CLI’s temporary DerivedData directories. Physical-device signing, install and launch remain separate milestones.

## Build and verify from source

On a Mac with full Xcode and the Go version in `go.mod`, run this from the repository root:

```sh
bash scripts/package-macos-app.sh
```

The script creates `dist/macos/AppRivet.app` and `dist/macos/AppRivet-macOS.zip`. Use a new output folder as the first argument for a repeat package. The hosted simulator job verifies the archived bundle and invokes the same workflow as the window to create a project, build it, install it and launch it. The smoke test exercises the command path on a hosted Apple silicon Mac. Visual interaction and Intel execution require a Mac with the corresponding setup.
