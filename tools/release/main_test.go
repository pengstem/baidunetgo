package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelOf(t *testing.T) {
	for _, tt := range []struct {
		commits []commit
		want    string
	}{
		{[]commit{{subject: "Fix the grid"}}, "patch"},
		{[]commit{{subject: "Fix the grid"}, {subject: "Add sharing #minor"}}, "minor"},
		{[]commit{{subject: "Add sharing", body: "Changes the settings file. [major]"}, {subject: "x #minor"}}, "major"},
	} {
		if got := levelOf(tt.commits); got != tt.want {
			t.Errorf("levelOf(%v) = %s, want %s", tt.commits, got, tt.want)
		}
	}
}

func TestNotesFrom(t *testing.T) {
	got := notesFrom(parseLog("Fix the grid's border\x1f\x1e\nAdd sharing #minor\x1fbody\x1e\nRelease v0.1.0\x1f\x1e\nRestyle the interface\x1f\x1e\nAdds a toast\x1f\x1e"))
	want := "### 新增\n\n- Add sharing\n- Adds a toast\n\n### 修复\n\n- Fix the grid's border\n\n### 改进\n\n- Restyle the interface"
	if got != want {
		t.Errorf("notes:\n%s\nwant:\n%s", got, want)
	}
}

func TestChangelog(t *testing.T) {
	// A new file.
	c := addRelease("", "0.1.0", "2026-10-20", "- 首个版本")
	if unreleasedNotes(c) != "" {
		t.Errorf("unreleased notes of a fresh release: %q", unreleasedNotes(c))
	}
	// Notes written by hand become the next version's.
	c = strings.Replace(c, unreleasedHint, unreleasedHint+"\n\n- 手写的说明", 1)
	if got := unreleasedNotes(c); got != "- 手写的说明" {
		t.Errorf("unreleased notes = %q", got)
	}
	c = addRelease(c, "0.1.1", "2026-10-21", unreleasedNotes(c))
	want := "# 更新日志\n\n## Unreleased\n\n" + unreleasedHint + "\n\n" +
		"## [0.1.1] - 2026-10-21\n\n- 手写的说明\n\n" +
		"## [0.1.0] - 2026-10-20\n\n- 首个版本\n"
	if c != want {
		t.Errorf("changelog:\n%s\nwant:\n%s", c, want)
	}
	// A file without its title or an Unreleased section.
	c = addRelease("## [0.1.0] - 2026-10-20\n\n- a\n", "0.1.1", "2026-10-21", "- b")
	if !strings.HasPrefix(c, "## Unreleased\n") || !strings.Contains(c, "- b\n\n## [0.1.0] - 2026-10-20\n\n- a\n") {
		t.Errorf("changelog:\n%s", c)
	}
}

// TestRun releases a repository three times: its first version as
// mygo.json says, a patch, then a version set by hand.
func TestRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	sh := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	write := func(name, s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	release := func(want string) {
		t.Helper()
		t.Setenv("GITHUB_OUTPUT", filepath.Join(dir, "out"))
		os.Remove("out")
		if err := run("auto", "notes.md", "CHANGELOG.md", "mygo.json"); err != nil {
			t.Fatal(err)
		}
		out, _ := os.ReadFile("out")
		if !strings.Contains(string(out), "version="+want+"\n") {
			t.Fatalf("outputs %q, want version %s", out, want)
		}
		cfg, _ := os.ReadFile("mygo.json")
		if !strings.Contains(string(cfg), `"version": "`+want+`"`) {
			t.Fatalf("mygo.json: %s", cfg)
		}
		sh("git", "add", "-A")
		sh("git", "commit", "-qm", "Release v"+want)
		sh("git", "tag", "v"+want)
	}

	sh("git", "init", "-q")
	write("mygo.json", "{\n  \"name\": \"X\",\n  \"version\": \"0.1.0\"\n}\n")
	sh("git", "add", "-A")
	sh("git", "commit", "-qm", "Add everything")
	release("0.1.0")

	// Nothing since: nothing to release.
	t.Setenv("GITHUB_OUTPUT", filepath.Join(dir, "out"))
	os.Remove("out")
	if err := run("auto", "", "CHANGELOG.md", "mygo.json"); err != nil {
		t.Fatal(err)
	}
	if out, _ := os.ReadFile("out"); string(out) != "skip=true\n" {
		t.Fatalf("outputs %q, want skip", out)
	}

	write("a.txt", "a")
	sh("git", "add", "-A")
	sh("git", "commit", "-qm", "Fix a")
	release("0.1.1")
	notes, _ := os.ReadFile("notes.md")
	if string(notes) != "### 修复\n\n- Fix a\n" {
		t.Errorf("notes %q", notes)
	}

	cfg, _ := os.ReadFile("mygo.json")
	write("mygo.json", strings.Replace(string(cfg), "0.1.1", "1.0.0", 1))
	sh("git", "commit", "-qam", "Restyle everything")
	release("1.0.0")
}
