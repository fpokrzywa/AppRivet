# AppRivet frontends

Use the Windows or Linux launcher in your browser, or open **AppRivet.app** from Finder on macOS. Choose Flutter for one shared project across mobile, web, and desktop, or SwiftUI for native iOS. The Mac app includes the AppRivet command-line tool and integrates with Xcode’s iOS Simulator.

## Windows and Linux

Open the [AppRivet CI run](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml) and download the artifact for your operating system. Sign in to GitHub if prompted. The current Windows and Linux downloads target x86-64 desktop machines.

- **Windows**. Extract **AppRivet-Windows-download** and double-click **AppRivet.exe**. It opens your default browser without opening a Terminal window.
- **Linux**. Extract **AppRivet-Linux-download**, then extract **AppRivet-linux.tar.gz**. Open the **AppRivet** executable from your file manager. Use the file’s permissions dialog to allow execution if the file manager asks. The launcher uses `xdg-open` and requires a graphical desktop and a default browser.

Enter an idea under **Describe your app**, choose **Import brief** for a UTF-8 `.txt`, `.md` or `.markdown` file, or load a sample using **Try an example**. You can edit imported text before continuing. AppRivet does not cap brief length. Provider input limits apply when you send an AI request. Enter a project name, choose a creation method and click **Review prompt**. The browser launcher saves new projects in **AppRivet Projects** in your home folder. **Open Projects Folder** reveals the workspace so you can move the whole generated folder to a Mac. Use **Quit** to stop the launcher. The browser interface listens only on loopback.

For SwiftUI, **Automatic from brief** looks for checklist, notes, or counter terms and uses a blank starter when there is no single match. Flutter offers an offline starter or AI generation. AppRivet saves your description in `APP_BRIEF.md`.

### Reopen a project

Click a project name under **Your projects** in the browser. On macOS, choose **Your projects** and select its main folder, `APP_BRIEF.md`, or Xcode project. Selecting Flutter’s `ios/Runner.xcodeproj` or `ios/Runner.xcworkspace` reopens the surrounding Flutter project. AppRivet restores the brief, format, platforms, and saved creation settings. The browser’s **Open project folder** button reveals the selected files. The existing build controls work with the reopened project.

Use a different name to **Review as new project** if you want to create a separate copy from the restored settings. Older projects restore data from their saved brief and platform files. The original provider and model can be restored only when the project recorded those settings. API keys stay in Admin for the current session.

**Admin → Appearance** switches between the dark and light gradients for the current session. The header uses the ivory emblem on dark and the charcoal emblem on light.

### One Flutter project, selected platforms

1. Install the official Flutter SDK. AppRivet finds it on `PATH`, in `~/development/flutter` or `~/flutter`, or under the AppRivet toolchain cache. Set `APPRIVET_FLUTTER_SDK` to an absolute SDK directory to use a different location. A desktop launcher may need reopening after a PATH change. [Flutter installation](https://docs.flutter.dev/install/manual)
2. Choose **Flutter (shared code)** under **Project format**. Check the platforms you want under **App platforms**, then review and create the project. AppRivet invokes `flutter create` for exactly those platform folders. All targets share `lib/main.dart` and `pubspec.yaml`.
3. In **Your projects**, select a build target and click **Build**. In the Mac app, select it under **Build platform**. Review the source and confirm you trust the project’s build scripts. Web, Android and desktop builds display their output location. The Mac app can launch the iOS debug build in a selected Simulator.

After building in the Mac app, **Open in Browser** previews the built Web app on your Mac while the project remains open. **Show APK in Finder** reveals the Android build. Drag the APK onto a running Android Studio emulator and open the app there. **Show Simulator** brings the iOS Simulator back to the foreground after a successful iOS build and launch. [Installing an APK in the Android Emulator](https://developer.android.com/studio/run/emulator-install-add-files)

| Target | Build host and tools |
| --- | --- |
| Web | Windows, macOS, or Linux with Flutter |
| Android | Windows, macOS, or Linux with Flutter and Android SDK tools |
| iOS Simulator | macOS with Flutter, Xcode, and an installed iOS runtime |
| Windows | Windows with Flutter and Visual Studio C++ desktop tools |
| macOS | macOS with Flutter and Xcode |
| Linux | Linux with Flutter and desktop development libraries |

Copy the complete Flutter project folder to a matching host to build another target. AppRivet creates web output, debug Android APKs, desktop builds, and iOS Simulator builds. Store signing and publication are separate. [Official target setup](https://docs.flutter.dev/platform-integration)

### Generate with AI

1. Open **Admin** between **Your projects** and **Quit**. Choose OpenAI or Anthropic and a model. Presets include GPT-6.2 Sol, GPT-6.2 Astra, Claude Opus 5.5 and Claude Fable 5.2. **Custom model ID** accepts another model on the selected provider that supports structured output.
2. Paste a key or choose **Import key file** to read a `.txt` file containing one key. Use **Save settings** to select that provider and model and add or replace its key. Keys are held in memory separately for each provider. Closing or reloading the browser tab, or quitting the Mac app, clears the session settings. Provider API usage is billed to your API account.
3. Under **Creation method**, choose **Generate with AI**, then **Review prompt**. Check the provider, model, selected targets and full text, or copy the prompt. **Send & create project** sends that exact text and saves the returned SwiftUI or Dart source. AppRivet reads the brief file locally and sends the reviewed text.
4. Review the generated source before building. Generation fills one source file in a fixed project. The compiler reports any errors. Complex apps can require source fixes, additional files, or target-specific integrations.

The automated provider tests use mock HTTP responses. Live generation requires an API key with access to the selected model and is a separate validation step.

If the provider reaches its output limit, simplify what the first version must do. A short description can still ask for more code than the model can return in one response. AppRivet rejects incomplete source and lets you revise the brief or select a different model.

## macOS

### Install AppRivet

1. Download **AppRivet-macOS.zip** from GitHub Releases. The download works without signing in. [Verified Mac download](https://github.com/fpokrzywa/AppRivet/releases/download/v0.4.6/AppRivet-macOS.zip) · [Release notes](https://github.com/fpokrzywa/AppRivet/releases/tag/v0.4.6)
2. Extract **AppRivet-macOS.zip** and move **AppRivet.app** to Applications. The archive includes Apple silicon and Intel executables.
3. Open **AppRivet.app** from Finder. This preview uses an ad hoc signature and has no Apple notarization. If macOS says it couldn’t verify AppRivet is free of malware, click **Done**. If you trust the download, open **System Settings → Privacy & Security**, scroll to **Security**, and click **Open Anyway** for AppRivet. Authenticate if prompted and confirm with **Open**. [Apple’s Gatekeeper guidance](https://support.apple.com/en-us/102445)

### Update AppRivet on your Mac

Version 0.4.5 introduced in-app updates. In 0.4.5, open the **AppRivet** menu in the macOS menu bar while AppRivet is active and choose **Check for Updates…**. Starting in 0.4.6, the same button appears in the main window. To move from 0.4.4 or earlier, quit AppRivet and replace the copy in Applications with the download above. AppRivet also checks periodically. When a release is available, choose **Install Update**, then **Install and Relaunch** when prompted.

The updater verifies the download with AppRivet’s release key before unpacking it. It replaces the app in Applications. Project folders stay where you saved them. Finish active work and copy any unsaved brief before approving a relaunch. Re-enter your provider API key in **Admin** after reopening, as with any fresh session.

### Set up the iOS Simulator

Install full Xcode and open it once to finish setup. In **Xcode → Settings → Components**, install an iOS Simulator runtime compatible with the generated app’s iOS 17 minimum. Under **Settings → Locations → Command Line Tools**, select your Xcode installation. Runtime availability depends on your Xcode release. [Get Xcode](https://developer.apple.com/xcode/) · [Xcode system requirements](https://developer.apple.com/xcode/system-requirements/) · [Apple’s Simulator setup guide](https://developer.apple.com/documentation/xcode/downloading-and-installing-additional-xcode-components)

### Create and run an app on the Mac

1. Write your idea under **Describe your app** or use **Import brief**. Enter an app name such as `MyFirstApp` and choose a save folder.
2. Choose **SwiftUI (native iOS)** for an iPhone or iPad app. Choose **Flutter (shared code)** for multiple platforms and check the targets you want. Creating a Flutter project requires the Flutter SDK. [Flutter setup](https://docs.flutter.dev/install/manual)
3. To generate code from your description, open **Admin**, choose OpenAI or Anthropic and a model, and enter your provider API key. Then select **Generate with AI**, click **Review prompt**, and choose **Send & create project**. To try an iOS Simulator build without an API key, choose a SwiftUI starter instead. [AI setup and key handling](#generate-with-ai)
4. To run an iOS project, review the generated source, select an available iPhone or iPad simulator, confirm that you trust the project to build and run, and click **Build & Run**. If no simulator appears, check the runtime in Xcode and click **Refresh** in AppRivet. If building or launching fails, expand **Build details** for the command output.
5. Use **Your projects** to reopen a project. Use **Open in Xcode** to edit a SwiftUI project or **Open project folder** to access the generated Flutter code.

CI started the updater, reopened a Flutter project from its Runner package, served a Web preview, and used the packaged Mac app to build and launch a SwiftUI checklist on a hosted iPhone Simulator. A live AI generation run with an actual provider key still needs verification. [Passing CI run](https://github.com/fpokrzywa/AppRivet/actions/runs/45645873257)

Build scripts run with the access of your macOS user account. SwiftUI build output is kept in the CLI’s temporary DerivedData directories. Physical-device signing, install and launch remain separate milestones.

## Build and verify from source

On a Mac with full Xcode and the Go version in `go.mod`, run this from the repository root:

```sh
bash scripts/package-macos-app.sh
```

The script creates `dist/macos/AppRivet.app` and `dist/macos/AppRivet-macOS.zip`. Use a new output folder as the first argument for a repeat package. The hosted simulator job verifies the archived bundle and invokes the same workflow as the window to create a project, build it, install it and launch it. The smoke test exercises the command path on a hosted Apple silicon Mac. Visual interaction and Intel execution require a Mac with the corresponding setup.

See the build results and platform downloads under **Artifacts** in GitHub Actions. [Latest AppRivet CI runs](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml)

### Publish a Mac update

1. Increment `CFBundleVersion` and `CFBundleShortVersionString` in `desktop/Info.plist`. Wait for CI to pass on that commit and download its `AppRivet-desktop-download` artifact. Keep the ZIP exactly as packaged so that the framework’s symlinks and signatures are preserved.
2. Sign that ZIP with the release seed that matches `SUPublicEDKey`. On macOS, use Sparkle’s `sign_update` with the release key in Keychain. On the Windows release host, read the DPAPI-protected seed from `%LOCALAPPDATA%\AppRivet\Signing\sparkle-seed.dpapi` and pipe it to the signing command from the repository root. Keep the release seed and its backup private. [Sparkle release signing](https://sparkle-project.org/documentation/publishing/)

   ```powershell
   $keyStore = Join-Path $env:LOCALAPPDATA 'AppRivet/Signing/sparkle-seed.dpapi'
   $seed = Get-Content -Raw -LiteralPath $keyStore | ConvertTo-SecureString | ConvertFrom-SecureString -AsPlainText
   $plist = [xml](Get-Content -Raw desktop/Info.plist)
   $publicKey = $plist.SelectSingleNode('//key[.="SUPublicEDKey"]/following-sibling::*[1]').InnerText
   $seed | go run ./scripts/macos-update sign $publicKey 'path/to/AppRivet-macOS.zip'
   Remove-Variable seed
   ```

3. Publish the ZIP in GitHub Releases. Verify that the public download matches the archive you signed. Update `updates/appcast.xml` with the public ZIP URL, signature and length, the bundle build number and display version, and the minimum macOS version. Commit the feed to `main` after the asset is public.
4. Check for the release from an older installed build. Confirm that the signed download installs and relaunches, and reopen an existing project. CI verifies the packaged updater starts and tests an iOS Simulator build. In-place updates on real Macs, including Apple silicon and Intel, still need live validation.
