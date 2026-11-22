package admission

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

func ParsePolicy(b []byte) (Policy, error) {
	var p Policy
	if err := strictJSON(b, &p); err != nil {
		return Policy{}, invalidPolicy()
	}
	if err := validatePolicy(p); err != nil {
		return Policy{}, err
	}
	return p, nil
}
func ParseRequest(b []byte) (Request, error) {
	var r Request
	if err := strictJSON(b, &r); err != nil {
		return Request{}, invalidRequest()
	}
	if err := validateRequest(r); err != nil {
		return Request{}, err
	}
	return r, nil
}
func LoadPolicy(path string) (Policy, error) {
	b, err := readJSON(path)
	if err != nil {
		return Policy{}, err
	}
	return ParsePolicy(b)
}
func LoadRequest(path string) (Request, error) {
	b, err := readJSON(path)
	if err != nil {
		return Request{}, err
	}
	return ParseRequest(b)
}
func readJSON(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, failure("io_error", "policy/request must be an accessible regular file, not a symlink")
	}
	if info.Size() > MaxJSONBytes {
		return nil, failure("resource_limit", "policy/request exceeds 64 KiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, failure("io_error", "cannot read policy/request")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, failure("io_error", "policy/request identity changed")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxJSONBytes+1))
	if err != nil {
		return nil, failure("io_error", "cannot read policy/request")
	}
	if len(b) > MaxJSONBytes {
		return nil, failure("resource_limit", "policy/request exceeds 64 KiB")
	}
	return b, nil
}
func strictJSON(b []byte, out any) error {
	if len(b) > MaxJSONBytes {
		return invalidPolicy()
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := walkJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return invalidPolicy()
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func walkJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return invalidPolicy()
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t == nil {
		return invalidPolicy()
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return invalidPolicy()
			}
			for _, r := range name {
				if r < 32 || r > 126 {
					return invalidPolicy()
				}
			}
			name = strings.ToLower(name)
			if seen[name] {
				return invalidPolicy()
			}
			seen[name] = true
			if err = walkJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := walkJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return invalidPolicy()
	}
	_, err = d.Token()
	return err
}
