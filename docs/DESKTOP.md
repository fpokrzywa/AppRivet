# AppRivet frontends

Use the Windows or Linux launcher to create a SwiftUI project in your browser. On macOS, open **AppRivet.app** from Finder to create, build, and run the project in an iOS Simulator. The Mac app includes the AppRivet command-line tool. The simulator flow uses unsigned builds and works without a phone or developer signing team.

## Windows and Linux

Open the [AppRivet CI run](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml) and download the artifact for your operating system. Sign in to GitHub if prompted. The current Windows and Linux downloads target x86-64 desktop machines.

- **Windows**. Extract **AppRivet-Windows-download** and double-click **AppRivet.exe**. It opens your default browser without opening a Terminal window.
- **Linux**. Extract **AppRivet-Linux-download**, then extract **AppRivet-linux.tar.gz**. Open the **AppRivet** executable from your file manager. Use the file’s permissions dialog to allow execution if the file manager asks. The launcher uses `xdg-open` and requires a graphical desktop and a default browser.

Enter an idea under **Describe your app**, or load a sample using **Try an example**. Enter a project name and click **Create SwiftUI Starter**. The browser launcher saves the new project in **AppRivet Projects** in your home folder. **Open Projects Folder** reveals the workspace so you can move the whole generated folder to a Mac. Use **Quit** to stop the launcher when you finish. The browser interface listens only on loopback. Simulator builds and launches use the AppRivet Mac app with Xcode.

With **Automatic from brief**, AppRivet looks for checklist, notes, or counter terms. It creates a blank SwiftUI starter when there is no single match. The **Starter** picker lets you override that choice. The checklist saves items and completion, the notes starter saves a single editable note, and the counter saves its tally. AppRivet saves your full description in `APP_BRIEF.md`. Custom features require editing the SwiftUI source.

## macOS

1. Open a successful [AppRivet CI run](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml) and download **AppRivet-desktop-download** under **Artifacts**. Sign in to GitHub if prompted.
2. Extract the download, then extract **AppRivet-macOS.zip** inside it. Move **AppRivet.app** to Applications or another folder you use for apps. The archive includes Apple silicon and Intel executables.
3. Install full Xcode and open it once to finish setup. In **Xcode → Settings → Components**, install an iOS runtime compatible with the generated app’s iOS 17 minimum. Select the full Xcode installation under **Settings → Locations → Command Line Tools**. Exact runtime availability depends on your Xcode release. [Xcode system requirements](https://developer.apple.com/xcode/system-requirements/) · [Installing components](https://developer.apple.com/documentation/xcode/downloading-and-installing-additional-xcode-components)
4. Open **AppRivet.app**. The CI download uses an ad hoc signature and has no Apple notarization. If macOS blocks it as an unverified developer and you trust the download, macOS provides an **Open Anyway** option under **System Settings → Privacy & Security** after you try to open it. [Apple’s Gatekeeper guidance](https://support.apple.com/en-us/102445)

### Create and run an app on the Mac

1. Enter an idea under **Describe your app** and an app name such as `MyFirstApp`. Leave **Bundle ID** blank to use the default. Pick a save folder, then click **Create SwiftUI Starter**. AppRivet creates a new subfolder and refuses to overwrite an existing project. Use **Open Project…** to reopen a previous project.
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

All five jobs passed on `6627dd4`. The Mac download launched a generated SwiftUI app on an iPhone 16 simulator. The Windows and Linux downloads each passed the project creation smoke test. [Verified builds and downloads](https://github.com/fpokrzywa/AppRivet/actions/runs/45553338536#artifacts)
