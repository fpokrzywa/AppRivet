"""Exercise the packaged launcher's loopback workspace and clean shutdown."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import urllib.parse
import urllib.request


executable = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix="apprivet-studio-") as home:
    sdk = Path(home, "flutter-test-sdk")
    env = dict(os.environ, HOME=home, USERPROFILE=home, APPRIVET_FLUTTER_SDK=str(sdk))
    with subprocess.Popen([str(executable), "--serve-only"], env=env, stdout=subprocess.PIPE, text=True) as app:
        try:
            address = urllib.parse.urlsplit(app.stdout.readline().strip())
            assert address.scheme == "http" and address.hostname == "127.0.0.1" and address.fragment
            base = f"{address.scheme}://{address.netloc}"
            headers = {"X-AppRivet-Session": address.fragment}

            def request(path, body=None):
                return urllib.request.urlopen(urllib.request.Request(base + path, data=body, headers=headers), timeout=15)

            with request("/api/state") as state:
                capability = json.load(state)
                assert capability["project_formats"] == [] and not capability["flutter_sdk"]
            launcher = sdk / "bin" / ("flutter.bat" if os.name == "nt" else "flutter")
            launcher.parent.mkdir(parents=True)
            launcher.write_text("", encoding="utf-8")  # Detection must be passive.
            launcher.chmod(0o755)
            with request("/api/state") as state:
                capability = json.load(state)
                assert capability["project_formats"] == ["flutter"] and capability["flutter_sdk"]
            with request("/") as page:
                html = page.read()
                assert b"Describe your app" in html and b"<style>" in html
                assert b"<script>" in html and b"X-AppRivet-Session" in html
                assert b'class="brand-mark"' in html and b"<span>AppRivet</span>" in html and b'src="/apprivet-mark-light.png"' in html
                assert b'<select id="appearance">' in html
                assert b"radial-gradient(ellipse at 8% 0%" in html
                assert "img-src 'self'" in page.headers["Content-Security-Policy"]
                assert b"Import brief" in html and b"Review prompt" in html and b'aria-labelledby="admin-heading"' in html
                assert "style-src 'sha256-" in page.headers["Content-Security-Policy"]
            for tone in ("light", "dark"):
                with request(f"/apprivet-mark-{tone}.png") as mark:
                    assert mark.headers["Content-Type"] == "image/png"
                    assert mark.read(26)[25] == 6  # Truecolor PNG with alpha.
            brief = "A checklist I can use to pack for a weekend trip.\n" + "Store the list on the device.\n" * 2000 + "Show my trip plan."
            project = {"name": "BrowserSmoke", "brief": brief}
            with request("/api/review", json.dumps({**project, "provider": "openai", "model": "gpt-6.2-sol"}).encode()) as review:
                draft = json.load(review)
                assert draft["provider"] == "openai" and draft["model"] == "gpt-6.2-sol"
                assert len(draft["approval"]) == 64 and draft["prompt"].endswith(brief)
                assert not Path(home, "AppRivet Projects").exists()
            with request("/api/create", json.dumps(project).encode()) as created:
                data = json.load(created)
                assert created.status == 201 and data["suggested_scheme"] == "BrowserSmoke"
                assert data["starter"] == "checklist"
            work = Path(home, "AppRivet Projects", "BrowserSmoke")
            assert (work / "BrowserSmoke.xcodeproj" / "project.pbxproj").is_file()
            assert brief in (work / "APP_BRIEF.md").read_text(encoding="utf8")
            with request("/api/project", json.dumps({"name": "BrowserSmoke"}).encode()) as reopened:
                settings = json.load(reopened)["settings"]
                assert settings["name"] == "BrowserSmoke" and settings["brief"] == brief
                assert settings["format"] == "swiftui" and settings["platforms"] == ["ios"]
            with request("/api/state") as state:
                assert json.load(state)["projects"] == ["BrowserSmoke"]
            with request("/api/quit", b"") as stopped:
                assert stopped.status == 204
            assert app.wait(timeout=15) == 0
        finally:
            if app.poll() is None:
                app.kill()

print("AppRivet browser launcher detected Flutter, served the UI, created a project and quit cleanly.")
