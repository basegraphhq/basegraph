package process

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type goModule struct {
	ModulePath string
	Dir        string
}

func discoverGoModules(root string) ([]goModule, error) {
	entries := make(map[string]goModule)

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" {
				return fs.SkipDir
			}
			if strings.HasPrefix(name, ".") && len(name) > 1 {
				return fs.SkipDir
			}
			return nil
		}

		if d.Name() != "go.mod" {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read go.mod: %w", readErr)
		}

		modulePath, parseErr := modulePathFromFile(data)
		if parseErr != nil {
			return fmt.Errorf("parse go.mod: %w", parseErr)
		}

		dir := filepath.Dir(path)
		mod := goModule{
			ModulePath: modulePath,
			Dir:        dir,
		}
		entries[dir] = mod

		return nil
	}

	if err := filepath.WalkDir(root, walkFn); err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no go modules found under %s", root)
	}

	mods := make([]goModule, 0, len(entries))
	for _, mod := range entries {
		mods = append(mods, mod)
	}

	sort.Slice(mods, func(i, j int) bool {
		if mods[i].ModulePath == mods[j].ModulePath {
			return mods[i].Dir < mods[j].Dir
		}
		return mods[i].ModulePath < mods[j].ModulePath
	})

	return mods, nil
}

func modulePathFromFile(data []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return "", fmt.Errorf("invalid module directive: %q", line)
		}
		path := strings.Trim(fields[1], `"`)
		if path == "" {
			return "", fmt.Errorf("empty module path in directive: %q", line)
		}
		return path, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("module directive not found")
}
