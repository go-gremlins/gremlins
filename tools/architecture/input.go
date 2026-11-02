package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// decode rejects duplicate keys before typed decoding can silently replace them.
func decode(data []byte, target any) error {
	tokens := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueValue(tokens); err != nil {
		return fmt.Errorf("invalid JSON: %w; fix: use non-null values and unique keys", err)
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON; fix: retain one document")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("invalid fields: %w; fix: use the documented version 1 input", err)
	}

	return nil
}

func uniqueValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return fmt.Errorf("null value")
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for dec.More() {
		if delim == '{' {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid key %q", key)
			}
			seen[name] = true
		}
		if err := uniqueValue(dec); err != nil {
			return err
		}
	}
	_, err = dec.Token()

	return err
}

func localPath(root, name string) (string, error) {
	if !validPath(name) {
		return "", fmt.Errorf("unsafe path %q; fix: use a clean relative checkout path", name)
	}
	current := root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("cannot inspect %q; fix: restore the regular checkout file", name)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("linked path %q; fix: replace links with regular checkout files", name)
		}
	}

	return current, nil
}

func readLocal(root, name string) ([]byte, error) {
	p, err := localPath(root, name)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return nil, fmt.Errorf("invalid input file %q; fix: use a regular file under 1 MiB", name)
	}
	confined, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open checkout: %w", err)
	}
	defer confined.Close()
	data, err := confined.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", name, err)
	}

	return data, nil
}

func loadJSON(root, name string, target any) error {
	data, err := readLocal(root, name)
	if err != nil {
		return err
	}
	if err := decode(data, target); err != nil {
		return fmt.Errorf("input %q: %w", name, err)
	}

	return nil
}
