package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverGoModules(t *testing.T) {
	root := t.TempDir()

	writeGoMod(t, root, "module example.com/root\n\ngo 1.21\n")

	modA := filepath.Join(root, "services", "modA")
	if err := os.MkdirAll(modA, 0o755); err != nil {
		t.Fatalf("mkdir modA: %v", err)
	}
	writeGoMod(t, modA, "module example.com/root/services/modA\n\ngo 1.21\n")

	modB := filepath.Join(root, "lib", "modB")
	if err := os.MkdirAll(modB, 0o755); err != nil {
		t.Fatalf("mkdir modB: %v", err)
	}
	writeGoMod(t, modB, "module example.com/root/lib/modB\n\ngo 1.21\n")

	mods, err := discoverGoModules(root)
	if err != nil {
		t.Fatalf("discoverGoModules returned error: %v", err)
	}

	if len(mods) != 3 {
		t.Fatalf("expected 3 modules, got %d", len(mods))
	}

	want := map[string]string{
		"example.com/root":               root,
		"example.com/root/lib/modB":      modB,
		"example.com/root/services/modA": modA,
	}

	for _, mod := range mods {
		dir, ok := want[mod.ModulePath]
		if !ok {
			t.Fatalf("unexpected module path %s", mod.ModulePath)
		}
		if dir != mod.Dir {
			t.Fatalf("module %s dir mismatch: want %s, got %s", mod.ModulePath, dir, mod.Dir)
		}
		delete(want, mod.ModulePath)
	}

	if len(want) != 0 {
		t.Fatalf("modules not discovered: %v", want)
	}
}

func TestDiscoverGoModulesNoModules(t *testing.T) {
	root := t.TempDir()
	if _, err := discoverGoModules(root); err == nil {
		t.Fatalf("expected error when no modules present")
	}
}

func writeGoMod(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}
