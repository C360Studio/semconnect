package conformance

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHarnessVendorReuseRequiresExactCleanSource(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	runScript := readFile(t, filepath.Join(root, "conformance", "run.sh"))
	if !strings.Contains(runScript, `git_source_matches_commit "$SEMSTREAMS_VENDOR_DIR" "$SEMSTREAMS_COMMIT"`) {
		t.Fatal("ensure_semstreams_vendor does not bind vendor reuse to exact clean source identity")
	}
}

func TestHarnessETSVendorReuseRequiresExactCleanSource(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	runScript := readFile(t, filepath.Join(root, "conformance", "run.sh"))
	etsFunction := textBetween(t, runScript, "ensure_ets_vendor() {", "\n}\n\nensure_semstreams_vendor() {")
	required := []struct {
		fragment string
		behavior string
	}{
		{`git_source_matches_commit "$ETS_VENDOR_DIR" "$ETS_COMMIT"`,
			"bind vendor reuse to exact clean source identity"},
		{`[[ "$ETS_VENDOR_DIR" != "$SCRIPT_DIR/.vendor/ets" ]]`,
			"validate the exact harness-owned replacement path"},
		{`mktemp -d "$vendor_parent/.ets-refresh.XXXXXX"`,
			"materialize replacements outside the active vendor path"},
		{`git clone --filter=blob:none "$ETS_GIT_URL" "$refresh_dir/source"`,
			"retain a real Git checkout for Maven SCM metadata"},
		{`git_source_matches_commit "$refresh_dir/source" "$ETS_COMMIT"`,
			"validate materialized source before replacement"},
		{`mv "$refresh_dir/source" "$ETS_VENDOR_DIR"`,
			"replace the active vendor only after validation"},
	}
	for _, requirement := range required {
		if !strings.Contains(etsFunction, requirement.fragment) {
			t.Errorf("ensure_ets_vendor does not %s", requirement.behavior)
		}
	}
}

func TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift(t *testing.T) {
	root := repositoryRoot(t)
	helper := filepath.Join(root, "conformance", "lib", "vendor_identity.sh")
	if _, err := os.Stat(helper); err != nil {
		t.Fatalf("stat vendor identity helper: %v", err)
	}

	repository := filepath.Join(t.TempDir(), "source")
	run(t, "git", "init", "-q", repository)
	run(t, "git", "-C", repository, "config", "user.email", "test@example.invalid")
	run(t, "git", "-C", repository, "config", "user.name", "SemConnect Test")
	tracked := filepath.Join(repository, "tracked.go")
	if err := os.WriteFile(tracked, []byte("package source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, "git", "-C", repository, "add", "tracked.go")
	run(t, "git", "-C", repository, "commit", "-q", "-m", "fixture")
	commit := strings.TrimSpace(run(t, "git", "-C", repository, "rev-parse", "HEAD"))

	requireSourceMatch(t, helper, repository, commit, true)
	requireSourceMatch(t, helper, repository, strings.Repeat("0", 40), false)

	if err := os.WriteFile(tracked, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	requireSourceMatch(t, helper, repository, commit, false)
	run(t, "git", "-C", repository, "restore", "tracked.go")

	if err := os.WriteFile(filepath.Join(repository, "untracked.go"), []byte("package injected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	requireSourceMatch(t, helper, repository, commit, false)

	if err := os.Remove(filepath.Join(repository, "untracked.go")); err != nil {
		t.Fatal(err)
	}
	// rev-parse HEAD still succeeds with a corrupt index, while git status
	// exits non-zero. Source identity must fail closed rather than interpreting
	// the command's empty stdout as a clean worktree.
	if err := os.WriteFile(filepath.Join(repository, ".git", "index"), []byte("corrupt-index"), 0o600); err != nil {
		t.Fatal(err)
	}
	requireSourceMatch(t, helper, repository, commit, false)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Dir(filepath.Dir(filename))
}

func requireSourceMatch(t *testing.T, helper, repository, commit string, want bool) {
	t.Helper()
	command := exec.Command(
		"bash", "-c", `source "$1"; git_source_matches_commit "$2" "$3"`,
		"bash", helper, repository, commit,
	)
	err := command.Run()
	if got := err == nil; got != want {
		t.Fatalf("git_source_matches_commit(%q, %q) success = %t, want %t (err=%v)", repository, commit, got, want, err)
	}
}

func textBetween(t *testing.T, content, start, end string) string {
	t.Helper()
	startIndex := strings.Index(content, start)
	if startIndex < 0 {
		t.Fatalf("start marker %q not found", start)
	}
	endOffset := strings.Index(content[startIndex:], end)
	if endOffset < 0 {
		t.Fatalf("end marker %q not found after %q", end, start)
	}
	return content[startIndex : startIndex+endOffset]
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}
