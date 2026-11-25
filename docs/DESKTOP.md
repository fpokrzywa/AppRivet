# AppRivet frontends

Use the Windows or Linux launcher in your browser, or open **AppRivet.app** from Finder on macOS. Choose Flutter for one shared project across mobile, web, and desktop, or SwiftUI for native iOS. The Mac app includes the AppRivet command-line tool and integrates with Xcode’s iOS Simulator.

## Windows and Linux

The Windows and Linux launchers target x86-64 desktop machines. They open AppRivet in your default browser.

### Install on Windows

1. Download **AppRivet-Windows.zip**. [AppRivet 0.4.11 Windows ZIP](https://github.com/fpokrzywa/AppRivet/releases/download/v0.4.11/AppRivet-Windows.zip)
2. Right-click the downloaded ZIP and select **Extract All**.
3. Double-click **AppRivet.exe**. The browser opens your local workspace.

### Install on Linux

Open the [AppRivet Linux CI builds](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml), download **AppRivet-Linux-download** from a successful run, and extract **AppRivet-linux.tar.gz**. Linux downloads are identified by the CI run. Open the **AppRivet** executable from your file manager. Use the file’s permissions dialog to allow execution if the file manager asks. The launcher uses `xdg-open` and requires a graphical desktop and a default browser.

### Project formats and SDK detection

AppRivet shows Flutter when it detects the installed SDK. On Windows and Linux, install Flutter to enable project creation. Choose **Check again** after installing the SDK, or relaunch AppRivet after adding Flutter to Path. The browser lets you reopen and inspect existing projects while you set up the SDK. The Mac app also offers SwiftUI and discovers iOS Simulators through Xcode.

Enter an idea under **Describe your app**, choose **Import brief** for a UTF-8 `.txt`, `.md` or `.markdown` file, or load a sample using **Try an example**. You can edit imported text before continuing. AppRivet does not cap brief length. Provider input limits apply when you send an AI request. Enter a project name, choose a creation method and click **Review prompt**. The browser launcher saves new projects in **AppRivet Projects** in your home folder. **Open Projects Folder** reveals the workspace so you can move the whole generated folder to a Mac. Use **Quit** to stop the launcher. The browser interface listens only on loopback.

New SwiftUI and Flutter projects default to **Generate with AI**. Choose an offline starter to try the build tools with template code. The SwiftUI offline starter from brief picks a checklist, notes or counter template when it finds a match. AppRivet saves your description in `APP_BRIEF.md`.

### Reopen a project

Click a project name under **Your projects** in the browser. On macOS, choose **Your projects** and select its main folder, `APP_BRIEF.md`, or Xcode project. Selecting Flutter’s `ios/Runner.xcodeproj` or `ios/Runner.xcworkspace` reopens the surrounding Flutter project. AppRivet restores the brief, format, platforms, and saved creation settings. The browser’s **Open project folder** button reveals the selected files. The existing build controls work with the reopened project.

For a project marked as an offline starter, choose **Generate a new project from this brief** to keep your description and create a separate AI project. AppRivet suggests a new name, which you can edit. Build and Run compiles the selected project's saved source. Older projects restore data from their saved brief and platform files. AppRivet restores the original provider and model when the project recorded them.

**Admin → Appearance** switches between the dark and light gradients for the current session. The header uses the ivory emblem on dark and the charcoal emblem on light.

### One Flutter project, selected platforms

1. Install the official Flutter SDK. AppRivet finds it on `PATH`, in `~/develop/flutter`, `~/development/flutter` or `~/flutter`, or under the AppRivet toolchain cache. Set `APPRIVET_FLUTTER_SDK` to an absolute SDK directory to use a different location. A desktop launcher may need reopening after a PATH change. [Flutter installation](https://docs.flutter.dev/install/manual)
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

Copy the complete Flutter project folder to a matching host to build another target. AppRivet creates web output, debug Android APKs, desktop builds, and iOS Simulator builds. Open **Publish** for signed release packages. [Official target setup](https://docs.flutter.dev/platform-integration)

### Build for Store

1. Open **Admin**, then **Store Connections**. For iOS signing, save your Apple Developer Team ID. For Android, save the SHA-256 fingerprint of your upload certificate.
2. Open a project and choose **Publish**. Select iOS or Android, review the signing setup, and confirm that you trust the project’s build scripts. **Build for Store** prepares a local package and records its SHA-256 in the project’s release history.
3. Reveal the signed package from **Publish**. Upload it through the store console when ready.

On a Mac, configure Xcode signing for the project with the selected team and an App Store distribution signing identity and profile. AppRivet archives the project for a device and exports an App Store Connect `.ipa`. The archive uses your local signing setup. [Xcode distribution setup](https://developer.apple.com/documentation/xcode/distributing-your-app-for-beta-testing-and-releases)

For Google Play, configure the Flutter project’s Gradle release signing for your upload keystore. AppRivet builds a release `.aab` and checks its signature against the upload certificate fingerprint saved in Settings. Keep the signing keystore and password outside source control. [Flutter Android deployment](https://docs.flutter.dev/deployment/android) · [Play App Signing](https://support.google.com/googleplay/android-developer/answer/9842756)

The release packages and their records are saved in the user's local AppRivet data folder, outside the project. You can build with signing configured and set up the store connections when you are ready to check account access.

### Store connections

- **App Store Connect**. Save the API Key ID and import the `.p8` private key. Include the Issuer ID for a team key, or leave it blank for an individual key. **Test Connection** checks read access to the app list. The Xcode signing team is a separate setting. [App Store Connect API setup](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api/)
- **Google Play**. Import a Google Cloud service account `.json` key. Invite the service account in Play Console and grant it access to the relevant app and testing track. **Test Connection** verifies that Google issues a token. App and track permissions still need a live check. [Google Play Developer API setup](https://developers.google.com/android-publisher/getting_started)

Only identifiers and signing fingerprints go in the local settings file. Store credentials go into macOS Keychain, Windows Credential Manager, or Linux Secret Service when available. Use **Remove** in Settings to clear a saved credential. Revoke lost credentials in the corresponding store console.

### Generate with AI

1. Open **Admin** between **Your projects** and **Quit**. Choose OpenAI or Anthropic and a model. Presets include GPT-6.2 Sol, GPT-6.2 Astra, Claude Opus 5.5 and Claude Fable 5.2. **Custom model ID** accepts another model on the selected provider that supports structured output.
2. Select the provider and paste its key, or choose **Import key (.txt)**. Use a UTF-8 plain-text file with just the key on one line. Click **Save settings** to add or replace that provider’s key. **Remove key** clears the selected provider’s saved credential.
3. Keep **Generate with AI** under **Creation method** and choose **Review planning requests**. For a simple brief, you can turn off component planning and choose **Review AI prompt**. Check the provider, model, selected targets and full text. **Send & generate project** sends the approved single prompt and saves the returned SwiftUI or Dart source.
4. Review the generated source before building. The compiler reports any errors. Complex apps can require source fixes or target-specific integrations.

AppRivet stores OpenAI and Anthropic keys separately in macOS Keychain, Windows Credential Manager, or Linux Secret Service when available. If the browser launcher displays session storage, the key is cleared when the session ends. Provider API usage is billed to your API account.

The automated provider tests use mock HTTP responses. Live generation requires an API key with access to the selected model and is a separate validation step.

### Plan a complex app

**Plan components before generating code** is selected by default. For briefs over 4 KiB of UTF-8 text, AppRivet requires component planning to keep the requests bounded.

1. Review the planning prompts and request budget, then choose **Send to plan**. AppRivet splits very long briefs at text boundaries and synthesizes the extracted requirements into a plan. It saves the original brief in full.
2. Check the component responsibilities and shared interfaces against your brief. Review the exact code prompt, destination file and output-token cap for each component. Approve once to send all of the displayed code prompts. Input sizes are shown in UTF-8 bytes, and output caps are upper limits for each request.
3. Review the generated files and build the app. If a component fails, AppRivet names it and leaves the project uncreated. You can revise the brief or request a plan with smaller components.

## macOS

### Install AppRivet

1. Download and open the notarized **AppRivet-macOS.dmg**. The app includes Apple silicon and Intel executables. [AppRivet 0.4.11 macOS disk image](https://github.com/fpokrzywa/AppRivet/releases/download/v0.4.11/AppRivet-macOS.dmg) and [AppRivet 0.4.11 release notes](https://github.com/fpokrzywa/AppRivet/releases/tag/v0.4.11).
2. Quit AppRivet if it’s open. Drag **AppRivet.app** onto **Applications** in the disk image window. If Finder asks what to do with the existing copy, choose **Replace**.
3. Eject the disk image. Open AppRivet from **Finder → Applications** and check **AppRivet → About AppRivet** to confirm the installed version.

### Update AppRivet on your Mac

Version 0.4.5 introduced in-app updates. In 0.4.5, open the **AppRivet** menu in the macOS menu bar while AppRivet is active and choose **Check for Updates…**. Starting in 0.4.6, the same button appears in the main window. To move from 0.4.4 or earlier, quit AppRivet and replace the copy in Applications with the download above. AppRivet also checks periodically. When a release is available, choose **Install Update**, then **Install and Relaunch** when prompted.

The updater verifies the download with AppRivet’s release key before unpacking it. It replaces the app in Applications. Project folders stay where you saved them. Finish active work and copy any unsaved brief before approving a relaunch. If your key was set before credential storage was added, enter it once in **Admin** and save it to the credential store.

If AppRivet fails to reopen, select it in **Finder → Applications** and choose **File → Get Info**. If the version matches the update you just approved, the replacement is on disk. Open it from Applications and record any macOS alert. If the version is still the old one, record the last updater message. You can use the disk image above to replace the app and get back to work. [Apple’s Finder guide](https://support.apple.com/guide/mac-help/mchlp1774/mac)

### Set up the iOS Simulator

Install full Xcode and open it once to finish setup. In **Xcode → Settings → Components**, install an iOS Simulator runtime compatible with the generated app’s iOS 17 minimum. Under **Settings → Locations → Command Line Tools**, select your Xcode installation. Runtime availability depends on your Xcode release. [Get Xcode](https://developer.apple.com/xcode/) · [Xcode system requirements](https://developer.apple.com/xcode/system-requirements/) · [Apple’s Simulator setup guide](https://developer.apple.com/documentation/xcode/downloading-and-installing-additional-xcode-components)

### Create and run an app on the Mac

1. Write your idea under **Describe your app** or use **Import brief**. Enter an app name such as `MyFirstApp` and choose a save folder.
2. Choose **SwiftUI (native iOS)** for an iPhone or iPad app. Choose **Flutter (shared code)** for multiple platforms and check the targets you want. Creating a Flutter project requires the Flutter SDK. [Flutter setup](https://docs.flutter.dev/install/manual)
3. To generate code from your description, open **Admin**, choose OpenAI or Anthropic and a model, and save your provider API key. Keep **Generate with AI** selected and review the planning requests and component plan. To try an iOS Simulator build without an API key, choose an offline SwiftUI starter. [AI setup and key handling](#generate-with-ai)
4. To run an iOS project, review the generated source, select an available iPhone or iPad simulator, confirm that you trust the project to build and run, and click **Build & Run**. If no simulator appears, check the runtime in Xcode and click **Refresh** in AppRivet. If building or launching fails, expand **Build details** for the command output.
5. Use **Your projects** to reopen a project. Use **Open in Xcode** to edit a SwiftUI project or **Open project folder** to access the generated Flutter code.

CI started the updater, reviewed a split brief, checked local publishing settings and history, reopened a Flutter project from its Runner package, served a Web preview, and used the packaged Mac app to build and launch a SwiftUI checklist on a hosted iPhone Simulator. A live AI generation run with an actual provider key still needs verification. [Passing CI run](https://github.com/fpokrzywa/AppRivet/actions/runs/45661501468)

Build scripts run with the access of your macOS user account. SwiftUI build output is kept in the CLI’s temporary DerivedData directories. Physical-device signing, install and launch remain separate milestones.

## Build and verify from source

On a Mac with full Xcode and the Go version in `go.mod`, run this from the repository root:

```sh
bash scripts/package-macos-app.sh
```

The script creates `dist/macos/AppRivet.app`, the drag-to-Applications installer `AppRivet-macOS.dmg` and the updater archive `AppRivet-macOS.zip`. Use a new output folder as the first argument for a repeat package. The hosted Mac job mounts the disk image, copies the app, and verifies the bundle signature. It also exercises the packaged app’s Simulator workflow on Apple silicon. Visual interaction and Intel execution require a Mac with the corresponding setup.

See the build results and platform downloads under **Artifacts** in GitHub Actions. [Latest AppRivet CI runs](https://github.com/fpokrzywa/AppRivet/actions/workflows/ci.yml)

### Publish a Mac update

1. Increment `CFBundleVersion` and `CFBundleShortVersionString` in `desktop/Info.plist` and merge the release commit to `main`. Wait for CI and download its `AppRivet-Windows-download` artifact. In **Actions → Sign and notarize a Mac release**, choose **Run workflow** on `main`, enter the new version, and uncheck **rehearsal**. Download `AppRivet-macOS-notarized-<version>` from the successful run. Use its DMG and versioned Sparkle ZIP for the public release. Keep the inner ZIP exactly as packaged so that the framework’s symlinks and signatures are preserved. [Mac release workflow](https://github.com/fpokrzywa/AppRivet/actions/workflows/macos-release.yml)
2. Sign that ZIP with the release seed that matches `SUPublicEDKey`. On macOS, use Sparkle’s `sign_update` with the release key in Keychain. On the Windows release host, read the DPAPI-protected seed from `%LOCALAPPDATA%\AppRivet\Signing\sparkle-seed.dpapi` and pipe it to the signing command from the repository root. Keep the release seed and its backup private. [Sparkle release signing](https://sparkle-project.org/documentation/publishing/)

   ```powershell
   $keyStore = Join-Path $env:LOCALAPPDATA 'AppRivet/Signing/sparkle-seed.dpapi'
   $seed = Get-Content -Raw -LiteralPath $keyStore | ConvertTo-SecureString | ConvertFrom-SecureString -AsPlainText
   $plist = [xml](Get-Content -Raw desktop/Info.plist)
   $publicKey = $plist.SelectSingleNode('//key[.="SUPublicEDKey"]/following-sibling::*[1]').InnerText
   $seed | go run ./scripts/macos-update sign $publicKey 'path/to/AppRivet-macOS.zip'
   Remove-Variable seed
   ```

3. Publish the Mac DMG, Mac ZIP and Windows ZIP in GitHub Releases. Verify that the public Mac ZIP matches the archive you signed. Update `updates/appcast.xml` with the public Mac ZIP URL, signature and length, the bundle build number and display version, and the minimum macOS version. In this guide, update the macOS and Windows download labels, release notes label, and their URLs to the same published version. Commit the feed and guide together to `main` after the assets are public. CI checks that the visible versions and link destinations match the feed.
4. Test a fresh DMG download with normal Gatekeeper settings. Check for the update from an older installed build, confirm it installs and relaunches, and reopen an existing project. The first transition from an ad hoc release to a Developer ID release needs a live update check. The hosted update smoke test exercised the 0.4.8 to 0.4.9 download, replacement and relaunch with an automated approval driver. [Hosted update test](https://github.com/fpokrzywa/AppRivet/actions/runs/45665268741)

### Sign and notarize AppRivet for direct Mac downloads

#### One-time Apple account setup

1. In the Apple Developer account, copy the **Team ID** from **Membership details**. The Account Holder creates a **Developer ID Application** certificate under **Certificates, Identifiers & Profiles**. Follow Apple's steps to create a certificate signing request on your Mac. After installing the certificate, use **Keychain Access → My Certificates** to export that certificate with its private key as a password-protected `.p12`. [Create a Developer ID certificate](https://developer.apple.com/help/account/certificates/create-developer-id-certificates)
2. In **App Store Connect → Users and Access → Integrations → App Store Connect API → Team Keys**, have the Account Holder request API access if needed. Generate a dedicated team key for AppRivet notarization with **Developer** access. Download its `.p8` and record the **Key ID** and **Issuer ID**. Apple lets you download the private key once. [App Store Connect team keys](https://developer.apple.com/help/app-store-connect/get-started/app-store-connect-api) and [Apple's notarytool authentication guide](https://developer.apple.com/documentation/technotes/tn3147-migrating-to-the-latest-notarization-tool).

#### GitHub setup

In **AppRivet → Settings → Environments → macos-release**, add the values below. The environment is restricted to `main`. Put credentials directly in GitHub's environment settings and keep the original files in secure storage. [Repository environment](https://github.com/fpokrzywa/AppRivet/settings/environments) and [GitHub environment secrets](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments).

| Kind | Name | Value |
| --- | --- | --- |
| Environment variable | `APPLE_TEAM_ID` | Apple Developer Team ID |
| Environment secret | `APPLE_DEVELOPER_ID_P12_BASE64` | Base64 of the exported `.p12`. On your Mac, `openssl base64 -A -in /path/to/DeveloperID.p12 \| pbcopy` copies it for pasting into GitHub. |
| Environment secret | `APPLE_DEVELOPER_ID_P12_PASSWORD` | Password chosen when you exported the `.p12` |
| Environment secret | `APPLE_NOTARY_KEY` | Full text of the `.p8`, including its BEGIN and END lines |
| Environment secret | `APPLE_NOTARY_KEY_ID` | Key ID from App Store Connect |
| Environment secret | `APPLE_NOTARY_ISSUER_ID` | Issuer ID from App Store Connect |

The Mac release workflow builds both Mac architectures, signs the app and its bundled helpers, submits the app and DMG to Apple, attaches both accepted tickets, and checks the results with Gatekeeper before uploading artifacts. **Rehearsal** runs the packaging checks without Apple credentials. Publishing the GitHub release and signing its updater ZIP with the existing Sparkle key remain the release steps above. [Apple distribution verification](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution) and [Sparkle release signing](https://sparkle-project.org/documentation/publishing/).
