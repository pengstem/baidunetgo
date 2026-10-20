// Command release prepares the next release: it picks the version, writes
// its section of CHANGELOG.md and sets it in mygo.json. The release
// workflow (.github/workflows/release.yml) runs it, then commits, tags and
// builds.
//
// The version is the last tag's, one patch higher. A commit since then that
// says #minor or #major raises that part instead, and a version set by hand
// in mygo.json above the last tag's is taken as it is. The first release is
// the version of mygo.json.
//
// The notes are what CHANGELOG.md has under "## Unreleased", when it has
// something there; otherwise they list the commits since the last tag.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	bump := flag.String("bump", "auto", "auto, patch, minor or major")
	notesOut := flag.String("notes", "", "write the release notes to this file")
	changelog := flag.String("changelog", "CHANGELOG.md", "the changelog")
	config := flag.String("config", "mygo.json", "the MyGo configuration")
	flag.Parse()
	if err := run(*bump, *notesOut, *changelog, *config); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(bump, notesOut, changelogPath, configPath string) error {
	cfg, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	m := configVersion.FindSubmatch(cfg)
	if m == nil {
		return fmt.Errorf("%s has no version", configPath)
	}
	current, err := parseVersion(string(m[2]))
	if err != nil {
		return fmt.Errorf("%s: %w", configPath, err)
	}

	last, _ := git("describe", "--tags", "--abbrev=0", "--match", "v[0-9]*")
	since := "HEAD"
	if last != "" {
		since = last + "..HEAD"
	}
	log, err := git("log", "--no-merges", "--format=%s%x1f%b%x1e", since)
	if err != nil {
		return err
	}
	commits := parseLog(log)

	changelog, err := os.ReadFile(changelogPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	unreleased := unreleasedNotes(string(changelog))

	if len(commits) == 0 && unreleased == "" && bump == "auto" {
		fmt.Println("nothing to release since", last)
		return output(map[string]string{"skip": "true"})
	}

	next := current
	if last != "" {
		base, err := parseVersion(last)
		if err != nil {
			return fmt.Errorf("tag %s: %w", last, err)
		}
		level := bump
		if level == "auto" {
			level = levelOf(commits)
		}
		next, err = base.bump(level)
		if err != nil {
			return err
		}
		if bump == "auto" && base.less(current) {
			next = current // set by hand
		}
	}

	notes := unreleased
	if notes == "" {
		notes = notesFrom(commits)
	}
	date := time.Now().UTC().Format("2006-01-02")
	if err := os.WriteFile(changelogPath, []byte(addRelease(string(changelog), next.String(), date, notes)), 0o644); err != nil {
		return err
	}
	cfg = configVersion.ReplaceAll(cfg, []byte(`${1}"`+next.String()+`"`))
	if err := os.WriteFile(configPath, cfg, 0o644); err != nil {
		return err
	}
	if notesOut != "" {
		if err := os.WriteFile(notesOut, []byte(notes+"\n"), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("release %s (last %s)\n\n%s\n", next, or(last, "none"), notes)
	return output(map[string]string{"skip": "false", "version": next.String(), "tag": "v" + next.String()})
}

var configVersion = regexp.MustCompile(`("version"\s*:\s*)"([^"]*)"`)

type version [3]int

func parseVersion(s string) (version, error) {
	var v version
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("version %q is not major.minor.patch", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("version %q is not major.minor.patch", s)
		}
		v[i] = n
	}
	return v, nil
}

func (v version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

func (v version) less(w version) bool {
	for i := range v {
		if v[i] != w[i] {
			return v[i] < w[i]
		}
	}
	return false
}

func (v version) bump(level string) (version, error) {
	switch level {
	case "major":
		return version{v[0] + 1, 0, 0}, nil
	case "minor":
		return version{v[0], v[1] + 1, 0}, nil
	case "patch":
		return version{v[0], v[1], v[2] + 1}, nil
	}
	return v, fmt.Errorf("unknown bump %q", level)
}

type commit struct{ subject, body string }

// parseLog reads git log --format=%s%x1f%b%x1e, leaving out the commits
// of earlier releases.
func parseLog(log string) []commit {
	var cs []commit
	for _, rec := range strings.Split(log, "\x1e") {
		subject, body, _ := strings.Cut(strings.TrimSpace(rec), "\x1f")
		subject = strings.TrimSpace(subject)
		if subject == "" || releaseCommit.MatchString(subject) {
			continue
		}
		cs = append(cs, commit{subject, strings.TrimSpace(body)})
	}
	return cs
}

var (
	releaseCommit = regexp.MustCompile(`^Release v\d`)
	marker        = regexp.MustCompile(`(?i)[#\[](major|minor|patch|skip ci|skip release)\]?`)
)

// levelOf is the largest part any commit asks to raise, patch by default.
func levelOf(cs []commit) string {
	level := "patch"
	for _, c := range cs {
		for _, m := range marker.FindAllStringSubmatch(c.subject+"\n"+c.body, -1) {
			switch strings.ToLower(m[1]) {
			case "major":
				return "major"
			case "minor":
				level = "minor"
			}
		}
	}
	return level
}

// notesFrom lists the commits under 新增, 修复 and 改进, by the verb each
// subject starts with, newest first as git log gives them.
func notesFrom(cs []commit) string {
	groups := []struct {
		title string
		verbs []string
		items []string
	}{
		{title: "新增", verbs: []string{"add", "support", "introduce", "implement"}},
		{title: "修复", verbs: []string{"fix", "repair", "revert"}},
		{title: "改进"},
	}
	for _, c := range cs {
		s := strings.Join(strings.Fields(marker.ReplaceAllString(c.subject, "")), " ")
		verb, _, _ := strings.Cut(strings.ToLower(s), " ")
		g := len(groups) - 1
		for i, grp := range groups[:g] {
			for _, v := range grp.verbs {
				if verb == v || verb == v+"s" || verb == v+"ed" || verb == v+"es" {
					g = i
				}
			}
		}
		groups[g].items = append(groups[g].items, "- "+s)
	}
	var b strings.Builder
	for _, g := range groups {
		if len(g.items) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("### " + g.title + "\n\n" + strings.Join(g.items, "\n"))
	}
	if b.Len() == 0 {
		return "- 维护更新"
	}
	return b.String()
}

const unreleasedHeading = "## Unreleased"

const unreleasedHint = "<!-- 写在这里的内容会成为下一个版本的更新说明；留空则按提交记录自动生成。 -->"

// unreleasedNotes is what the changelog has under "## Unreleased", without
// the hint, or "".
func unreleasedNotes(changelog string) string {
	_, body, ok := strings.Cut(changelog, unreleasedHeading+"\n")
	if !ok {
		return ""
	}
	if i := strings.Index(body, "\n## "); i >= 0 {
		body = body[:i]
	}
	return strings.TrimSpace(strings.ReplaceAll(body, unreleasedHint, ""))
}

// addRelease puts the version's section under an empty "## Unreleased",
// in place of what was there.
func addRelease(changelog, v, date, notes string) string {
	head := "# 更新日志\n\n"
	rest := ""
	if strings.HasPrefix(changelog, "## ") {
		head, rest = "", changelog
	} else if i := strings.Index(changelog, "\n## "); i >= 0 {
		head, rest = changelog[:i+1], changelog[i+1:]
	} else if strings.TrimSpace(changelog) != "" {
		head = strings.TrimRight(changelog, "\n") + "\n\n"
	}
	if strings.HasPrefix(rest, unreleasedHeading+"\n") {
		rest = rest[len(unreleasedHeading)+1:]
		if i := strings.Index(rest, "\n## "); i >= 0 {
			rest = rest[i+1:]
		} else {
			rest = ""
		}
	}
	section := fmt.Sprintf("## [%s] - %s\n\n%s\n", v, date, notes)
	if rest != "" {
		section += "\n" + rest
	}
	return head + unreleasedHeading + "\n\n" + unreleasedHint + "\n\n" + section
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// output writes the step's outputs for GitHub Actions, when there is a
// file for them.
func output(kv map[string]string) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	for k, v := range kv {
		fmt.Fprintf(f, "%s=%s\n", k, v)
	}
	return f.Close()
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
