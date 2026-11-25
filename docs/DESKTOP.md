# AppRivet frontends

Use the Windows or Linux launcher to create a SwiftUI project in your browser. On macOS, open **AppRivet.app** from Finder to create, build, and run the project in an iOS Simulator. The Mac app includes the AppRivet command-line tool. The simulator flow uses unsigned builds and works without a phone or developer signing team.

## Windows and Linux

Open the [AppRivet CI run](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml) and download the artifact for your operating system. Sign in to GitHub if prompted. The current Windows and Linux downloads target x86-64 desktop machines.

- **Windows**. Extract **AppRivet-Windows-download** and double-click **AppRivet.exe**. It opens your default browser without opening a Terminal window.
- **Linux**. Extract **AppRivet-Linux-download**, then extract **AppRivet-linux.tar.gz**. Open the **AppRivet** executable from your file manager. Use the file’s permissions dialog to allow execution if the file manager asks. The launcher uses `xdg-open` and requires a graphical desktop and a default browser.

Enter an idea under **Describe your app**, choose **Import brief** for a UTF-8 `.txt`, `.md` or `.markdown` file, or load a sample using **Try an example**. You can edit imported text before continuing. AppRivet does not cap brief length. Provider input limits apply when you send an AI request. Enter a project name, choose a creation method and click **Review prompt**. The browser launcher saves new projects in **AppRivet Projects** in your home folder. **Open Projects Folder** reveals the workspace so you can move the whole generated folder to a Mac. Use **Quit** to stop the launcher. The browser interface listens only on loopback.

With **Automatic from brief**, AppRivet looks for checklist, notes, or counter terms. It creates a blank SwiftUI starter when there is no single match. The **Creation method** picker can set a starter explicitly. AppRivet saves your description in `APP_BRIEF.md`.

### Generate with AI

1. Open **Admin** between **Your projects** and **Quit**. Choose OpenAI or Anthropic and a model. Presets include GPT-6.2 Sol, GPT-6.2 Astra, Claude Opus 5.5 and Claude Fable 5.2. **Custom model ID** accepts another model on the selected provider that supports structured output.
2. Paste a key or choose **Import key file** to read a `.txt` file containing one key. Use **Save settings** to select that provider and model and add or replace its key. Keys are held in memory separately for each provider. Closing or reloading the browser tab, or quitting the Mac app, clears the session settings. Provider API usage is billed to your API account.
3. Under **Creation method**, choose **Generate with AI**, then **Review prompt**. Check the provider, model, and full text, or copy the prompt. The review validates the input format and project settings locally. **Send & create project** sends that exact text to the selected API and puts the returned SwiftUI source into a new Xcode project. The brief file itself stays on your machine.
4. Open the new project on your Mac. Review the generated Swift source in Xcode before using **Build & Run**. Generation returns one Swift source file inside a fixed AppRivet project. Xcode compiles that source and reports any build errors. Generated apps can require source fixes or additional files for features beyond one self-contained SwiftUI view.

The automated provider tests use mock HTTP responses. Live generation requires an API key with access to the selected model and is a separate validation step.

## macOS

1. Open a successful [AppRivet CI run](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml) and download **AppRivet-desktop-download** under **Artifacts**. Sign in to GitHub if prompted.
2. Extract the download, then extract **AppRivet-macOS.zip** inside it. Move **AppRivet.app** to Applications or another folder you use for apps. The archive includes Apple silicon and Intel executables.
3. Install full Xcode and open it once to finish setup. In **Xcode → Settings → Components**, install an iOS runtime compatible with the generated app’s iOS 17 minimum. Select the full Xcode installation under **Settings → Locations → Command Line Tools**. Exact runtime availability depends on your Xcode release. [Xcode system requirements](https://developer.apple.com/xcode/system-requirements/) · [Installing components](https://developer.apple.com/documentation/xcode/downloading-and-installing-additional-xcode-components)
4. Open **AppRivet.app**. The CI download uses an ad hoc signature and has no Apple notarization. If macOS blocks it as an unverified developer and you trust the download, macOS provides an **Open Anyway** option under **System Settings → Privacy & Security** after you try to open it. [Apple’s Gatekeeper guidance](https://support.apple.com/en-us/102445)

### Create and run an app on the Mac

1. Write an idea under **Describe your app** or use **Import brief**. Enter an app name such as `MyFirstApp`, pick a save folder, and leave **Bundle ID** blank for the default. Choose a creation method, click **Review prompt** and confirm. AppRivet creates a new subfolder. Use **Your projects** to reopen a previous project.
2. Choose an available iPhone or iPad simulator. If the list is empty after first setup, check the runtime in Xcode and click **Refresh** in AppRivet.
3. Review the source and confirm that you trust the project to build and run, then click **Build & Run**. AppRivet builds the shared scheme matching the generated project name, boots the selected simulator if needed, installs the iOS Simulator product and launches the app. The first boot can take a few minutes.
4. Use **Open in Xcode** to edit the app. If build or launch fails, read the status message and expand **Build details** for command output.

Build scripts run with the access of your macOS user account. Build output is kept in the CLI’s temporary DerivedData directories. Physical-device signing, install and launch remain separate milestones.

## Build and verify from source

On a Mac with full Xcode and the Go version in `go.mod`, run this from the repository root:

```sh
bash scripts/package-macos-app.sh
```

The script creates `dist/macos/AppRivet.app` and `dist/macos/AppRivet-macOS.zip`. Use a new output folder as the first argument for a repeat package. The hosted simulator job verifies the archived bundle and invokes the same workflow as the window to create a project, build it, install it and launch it. The smoke test exercises the command path on a hosted Apple silicon Mac. Visual interaction and Intel execution require a Mac with the corresponding setup.

See the build results and platform downloads under **Artifacts** in GitHub Actions. [Latest AppRivet CI runs](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml)
