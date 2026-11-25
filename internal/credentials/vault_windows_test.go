package credentials

import (
	"crypto/rand"
	"testing"
)

func TestWindowsVaultPersistsFixtureByProvider(t *testing.T) {
	if NewVault() == nil {
		t.Skip("Windows Credential Manager cannot persist credentials in this logon session")
	}
	namespace := "AppRivet/Test/" + rand.Text() + "/"
	vault := windowsVault(namespace)
	for _, provider := range []string{"openai", "anthropic"} {
		t.Cleanup(func() {
			if err := vault.Remove(provider); err != nil {
				t.Errorf("remove isolated %s fixture: %v", provider, err)
			}
		})
	}
	if key, err := vault.Get("openai"); err != nil || key != "" {
		t.Fatal("a random fixture credential already exists", err)
	}
	if err := vault.Save("openai", "not-a-real-openai-key"); err != nil {
		t.Fatal("save OpenAI fixture", err)
	}
	if err := vault.Save("anthropic", "not-a-real-anthropic-key"); err != nil {
		t.Fatal("save Anthropic fixture", err)
	}
	if err := vault.Save("openai", "changed-openai-fixture"); err != nil {
		t.Fatal("replace OpenAI fixture", err)
	}
	reopened := windowsVault(namespace)
	if key, err := reopened.Get("openai"); err != nil || key != "changed-openai-fixture" {
		t.Fatal("the replacement fixture was not persisted", err)
	}
	if err := reopened.Remove("openai"); err != nil {
		t.Fatal("remove OpenAI fixture", err)
	}
	if key, err := vault.Get("openai"); err != nil || key != "" {
		t.Fatal("the removed fixture is still present", err)
	}
	if key, err := vault.Get("anthropic"); err != nil || key != "not-a-real-anthropic-key" {
		t.Fatal("removing one provider affected another provider", err)
	}
}
