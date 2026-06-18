package gitident

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// run executes a command in dir and fails the test on error.
func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// Keep git hermetic and quiet regardless of the host's global config.
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// initRepo creates a git repo at dir on branch `main` with one commit.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "init", "-b", "main", "-q")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-q", "-m", "init")
}

// eval resolves symlinks (macOS /var -> /private/var) for path comparison.
func eval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestResolve_OperaLayout(t *testing.T) {
	// ~/dev-vd/groundtruth/main + linked worktree ~/dev-vd/groundtruth/feat-x
	root := t.TempDir()
	mainWT := filepath.Join(root, "groundtruth", "main")
	initRepo(t, mainWT)
	run(t, mainWT, "git", "worktree", "add", "-q", "-b", "feat-x", "../feat-x")
	featWT := filepath.Join(root, "groundtruth", "feat-x")

	// Main worktree: project derived from the containing dir, not "main".
	id, err := Resolve(mainWT)
	if err != nil {
		t.Fatal(err)
	}
	if id.Project != "groundtruth" {
		t.Errorf("main: Project = %q, want groundtruth", id.Project)
	}
	if id.Instance != eval(t, mainWT) {
		t.Errorf("main: Instance = %q, want %q", id.Instance, eval(t, mainWT))
	}
	if id.Label != "main" {
		t.Errorf("main: Label = %q, want main", id.Label)
	}

	// Linked worktree: same project, distinct instance, branch label.
	id2, err := Resolve(featWT)
	if err != nil {
		t.Fatal(err)
	}
	if id2.Project != "groundtruth" {
		t.Errorf("feat-x: Project = %q, want groundtruth", id2.Project)
	}
	if id2.Instance != eval(t, featWT) {
		t.Errorf("feat-x: Instance = %q, want %q", id2.Instance, eval(t, featWT))
	}
	if id2.Label != "feat-x" {
		t.Errorf("feat-x: Label = %q, want feat-x", id2.Label)
	}

	// The whole point: the two worktrees of one repo share a project but have
	// different allocation keys.
	if id.Instance == id2.Instance {
		t.Fatal("worktrees must have distinct Instance paths")
	}
}

func TestResolve_ConventionalLayout(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "groundtruth")
	initRepo(t, repo)

	id, err := Resolve(repo)
	if err != nil {
		t.Fatal(err)
	}
	if id.Project != "groundtruth" {
		t.Errorf("Project = %q, want groundtruth", id.Project)
	}
	if id.Instance != eval(t, repo) {
		t.Errorf("Instance = %q, want %q", id.Instance, eval(t, repo))
	}
}

func TestResolve_FromSubdir(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "proj", "main")
	initRepo(t, repo)
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := Resolve(sub)
	if err != nil {
		t.Fatal(err)
	}
	if id.Project != "proj" {
		t.Errorf("Project = %q, want proj", id.Project)
	}
	if id.Instance != eval(t, repo) {
		t.Errorf("Instance = %q, want repo toplevel %q", id.Instance, eval(t, repo))
	}
}

func TestResolve_NotAGitRepo(t *testing.T) {
	if _, err := Resolve(t.TempDir()); err == nil {
		t.Fatal("expected error outside a git repo")
	}
}
