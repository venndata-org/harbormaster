// Package gitident derives a checkout's identity from git, worktree-aware. The
// allocation key is the instance path (this checkout's toplevel), which is what
// lets multiple worktrees of one repo run at once. See SPEC.md §2/§6.
package gitident

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Identity is the git-derived identity of a checkout.
type Identity struct {
	// Project is a default project name, stable across a repo's worktrees. It is
	// only a default: an explicit name in harbormaster.toml takes precedence (the
	// CLI applies that). See DECISIONS.md D6.
	Project string
	// Instance is the absolute path of this checkout (worktree toplevel). It is
	// the allocation key.
	Instance string
	// Label is a human-friendly name: the current branch, or the checkout's
	// directory basename when detached.
	Label string
	// RepoRoot is where harbormaster.toml lives. It equals Instance — committed
	// files are checked out into each worktree.
	RepoRoot string
}

// Resolve derives identity by running git in dir.
func Resolve(dir string) (Identity, error) {
	top, err := gitIn(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Identity{}, fmt.Errorf("not a git repository (or git not installed): %w", err)
	}

	commonDir, err := gitIn(top, "rev-parse", "--git-common-dir")
	if err != nil {
		return Identity{}, err
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(top, commonDir)
	}
	commonDir = filepath.Clean(commonDir)

	project := projectName(mainWorktreeTop(commonDir))

	label, _ := gitIn(dir, "branch", "--show-current")
	if label == "" {
		label = filepath.Base(top)
	}

	return Identity{
		Project:  project,
		Instance: top,
		Label:    label,
		RepoRoot: top,
	}, nil
}

// mainWorktreeTop returns the toplevel of the repo's main worktree given its git
// common dir (typically <mainTop>/.git).
func mainWorktreeTop(commonDir string) string {
	if filepath.Base(commonDir) == ".git" {
		return filepath.Dir(commonDir)
	}
	// Bare repo or unusual layout: best-effort parent.
	return filepath.Dir(commonDir)
}

// projectName derives a stable project name from the main worktree's toplevel.
//
// For a conventional clone (~/code/groundtruth) this is the basename. For the
// opera layout (~/dev-vd/groundtruth/main, a worktree-per-branch sibling tree)
// the basename is the generic worktree name "main"/"master", so we go up one
// level to the containing project directory. An explicit harbormaster.toml name
// overrides all of this. See DECISIONS.md D6.
func projectName(mainTop string) string {
	name := filepath.Base(mainTop)
	if name == "main" || name == "master" {
		if parent := filepath.Base(filepath.Dir(mainTop)); isRealName(parent) {
			return parent
		}
	}
	return name
}

func isRealName(s string) bool {
	switch s {
	case "", ".", string(filepath.Separator):
		return false
	}
	return true
}

func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}
