package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDesktopInstallLinksMatchPublishedVersion(t *testing.T) {
	feed, err := os.ReadFile("../../updates/appcast.xml")
	if err != nil {
		t.Fatal(err)
	}
	var appcast struct {
		Items []struct {
			Version string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(feed, &appcast); err != nil {
		t.Fatal(err)
	}
	if len(appcast.Items) == 0 || strings.TrimSpace(appcast.Items[0].Version) == "" {
		t.Fatal("the latest appcast item must declare the published version")
	}
	version := strings.TrimSpace(appcast.Items[0].Version)

	guide, err := os.ReadFile("../../docs/DESKTOP.md")
	if err != nil {
		t.Fatal(err)
	}
	const releases = "https://github.com/fpokrzywa/AppRivet/releases"
	for _, link := range []struct{ label, url string }{
		{"macOS disk image", releases + "/download/v" + version + "/AppRivet-macOS.dmg"},
		{"Windows ZIP", releases + "/download/v" + version + "/AppRivet-Windows.zip"},
		{"release notes", releases + "/tag/v" + version},
	} {
		want := fmt.Sprintf("[AppRivet %s %s](%s)", version, link.label, link.url)
		if !strings.Contains(string(guide), want) {
			t.Errorf("docs/DESKTOP.md is missing the published release link %s", want)
		}
	}
}
