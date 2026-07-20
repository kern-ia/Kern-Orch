package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	sd := filepath.Join(dir, name)
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadParsesFrontmatterTypeAndDescription(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "planner", "---\nname: planner\ntype: agent\ndescription: plans work\n---\n# Planner\nbody\n")
	writeSkill(t, dir, "sum", "---\nname: sum\ntype: tool\ndescription: adds numbers\n---\nbody\n")

	reg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg.Len() != 2 {
		t.Fatalf("len = %d; want 2", reg.Len())
	}
	planner, ok := reg.Get("planner")
	if !ok || planner.Type != TypeAgent || planner.Description != "plans work" {
		t.Fatalf("planner = %+v ok=%v", planner, ok)
	}
	sum, _ := reg.Get("sum")
	if sum.Type != TypeTool {
		t.Fatalf("sum type = %q; want tool", sum.Type)
	}
}

func TestLoadDefaultsNameFromDirAndRejectsBadType(t *testing.T) {
	dir := t.TempDir()
	// missing name -> defaults to directory name; bad type -> error
	writeSkill(t, dir, "weird", "---\ntype: gadget\n---\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestLoadMissingTypeErrors(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "notype", "---\nname: notype\ndescription: x\n---\n")
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestLoadEmptyDirIsEmptyRegistry(t *testing.T) {
	reg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg.Len() != 0 {
		t.Fatalf("len = %d; want 0", reg.Len())
	}
}

func TestListIsSortedByName(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "zeta", "---\nname: zeta\ntype: tool\n---\n")
	writeSkill(t, dir, "alpha", "---\nname: alpha\ntype: agent\n---\n")
	reg, _ := Load(dir)
	list := reg.List()
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "zeta" {
		t.Fatalf("List not sorted: %+v", list)
	}
}
