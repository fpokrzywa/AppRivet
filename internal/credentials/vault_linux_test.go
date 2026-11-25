package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxVaultUsesScopedSecretToolAndWritesKeyToStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("APPRIVET_TEST_VAULT", dir)
	const script = `#!/bin/sh
set -eu
verb=$1
shift
if [ "$verb" = "store" ]; then
  [ "$1" = "--label=AppRivet API key (openai)" ]
  shift
fi
[ "$#" = 4 ] && [ "$1" = "service" ] && [ "$2" = "com.github.fpokrzywa.AppRivet.ProviderKey" ] && [ "$3" = "provider" ] || exit 2
case "$4" in
  probe-*) [ "$verb" = "lookup" ] && exit 1 ;;
  openai|anthropic) ;;
  *) exit 2 ;;
esac
file="$APPRIVET_TEST_VAULT/$4.fixture"
case "$verb" in
  lookup) [ -f "$file" ] || exit 1; /bin/cat "$file"; printf '\n' ;;
  store) /bin/cat > "$file" ;;
  clear) /bin/rm -- "$file" ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "secret-tool"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	vault := NewVault()
	if vault == nil {
		t.Fatal("could not detect a working Secret Service fixture")
	}
	if err := vault.Save("openai", "not-a-real-openai-key"); err != nil {
		t.Fatal("save fixture", err)
	}
	if key, err := vault.Get("openai"); err != nil || key != "not-a-real-openai-key" {
		t.Fatal("invalid fixture round trip", err)
	}
	if key, err := vault.Get("anthropic"); err != nil || key != "" {
		t.Fatal("unexpected credential for other provider", err)
	}
	if err := vault.Remove("openai"); err != nil {
		t.Fatal("remove fixture", err)
	}
	if key, err := vault.Get("openai"); err != nil || key != "" {
		t.Fatal("removed fixture was returned", err)
	}
	if err := vault.Remove("openai"); err != nil {
		t.Fatal("missing item returned an error", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret-tool"), []byte("#!/bin/sh\necho 'unavailable' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if NewVault() != nil {
		t.Fatal("unavailable Secret Service was reported as available")
	}
}
