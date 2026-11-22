package snapshot

import (
	"path"
	"sort"
	"strings"
)

type pathRecord struct {
	spelling string
	file     bool
}
type pathSet map[string]pathRecord

func (s pathSet) add(name string) error {
	unsafe := func() error {
		return fail("unsafe_path", "file paths must be unique portable relative ASCII paths without links, traversal, reserved names or case conflicts")
	}
	if name == "" || len(name) > 180 {
		return unsafe()
	}
	for _, r := range name {
		if r < 32 || r > 126 || strings.ContainsRune(`<>:"\|?*`, r) {
			return unsafe()
		}
	}
	parts := strings.Split(name, "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." || len(part) > 80 || strings.TrimSpace(part) != part || strings.HasSuffix(part, ".") {
			return unsafe()
		}
		lower := strings.ToLower(part)
		base := strings.TrimRight(strings.SplitN(lower, ".", 2)[0], " ")
		if base == "con" || base == "prn" || base == "aux" || base == "nul" || base == "conin$" || base == "conout$" || (len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9') {
			return unsafe()
		}
		if lower == ".git" || lower == ".svn" || lower == ".hg" || lower == ".ssh" || lower == ".aws" || lower == ".azure" || lower == ".env" || strings.HasPrefix(lower, ".env.") || lower == "id_rsa" || lower == "id_ed25519" || lower == "id_ecdsa" || lower == "id_dsa" {
			return fail("sensitive_path", "selection includes a credential or VCS path; it was not packaged")
		}
		switch strings.ToLower(path.Ext(part)) {
		case ".p8", ".p12", ".pfx", ".pem", ".key", ".mobileprovision":
			return fail("sensitive_path", "selection includes a credential or signing-material filename; it was not packaged")
		}
		prefix := strings.Join(parts[:i+1], "/")
		key := strings.ToLower(prefix)
		leaf := i == len(parts)-1
		if old, ok := s[key]; ok {
			if old.spelling != prefix || old.file || leaf {
				return unsafe()
			}
		} else {
			s[key] = pathRecord{prefix, leaf}
		}
	}
	return nil
}
func selection(files []string) ([]string, error) {
	if len(files) == 0 {
		return nil, fail("invalid_arguments", "select at least one source file")
	}
	if len(files) > MaxFiles {
		return nil, limit()
	}
	ordered := append([]string{}, files...)
	sort.Strings(ordered)
	seen := pathSet{}
	for _, name := range ordered {
		if err := seen.add(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
