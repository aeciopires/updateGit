package git

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// buildTree creates:
//
//	base/
//	├── level1/            (repo, with sub/nested repo inside: never found)
//	├── group/             (dir)
//	│   ├── level2/        (repo)
//	│   └── org/           (dir)
//	│       ├── level3/    (repo)
//	│       └── team/      (dir)
//	│           ├── level4 (repo)
//	│           └── x/     (dir: level5 inside it, level6 in x/y)
//	│               └── y/z/level7 (repo: never found, the maximum depth is 6)
//	├── node_modules/pkg   (repo inside excluded dir)
//	├── .hidden/repo       (repo inside hidden dir)
//	└── link -> group      (symlink, not followed)
func buildTree(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	for _, repo := range []string{
		"level1", "group/level2", "group/org/level3", "group/org/team/level4",
		"group/org/team/x/level5", "group/org/team/x/y/level6", "group/org/team/x/y/z/level7",
		"node_modules/pkg", ".hidden/repo",
	} {
		initRepo(t, filepath.Join(base, repo), "main")
	}
	initRepo(t, filepath.Join(base, "level1", "sub", "nested"), "main")
	if err := os.Symlink(filepath.Join(base, "group"), filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	return base
}

func repoNames(repos []Repository) []string {
	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, r.Name)
	}
	return names
}

func TestFindRepositoriesDepth(t *testing.T) {
	setupGitEnv(t)
	base := buildTree(t)

	tests := []struct {
		depth int
		want  []string
	}{
		{1, []string{"level1"}},
		{2, []string{"group/level2", "level1"}},
		{3, []string{"group/level2", "group/org/level3", "level1"}},
		{4, []string{"group/level2", "group/org/level3", "group/org/team/level4", "level1"}},
		{5, []string{"group/level2", "group/org/level3", "group/org/team/level4", "group/org/team/x/level5", "level1"}},
		{6, []string{"group/level2", "group/org/level3", "group/org/team/level4", "group/org/team/x/level5", "group/org/team/x/y/level6", "level1"}},
		// clamped to 6: level 7 is never found
		{7, []string{"group/level2", "group/org/level3", "group/org/team/level4", "group/org/team/x/level5", "group/org/team/x/y/level6", "level1"}},
		{0, []string{"level1"}}, // clamped to 1
	}
	for _, tt := range tests {
		res, err := DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: tt.depth, ExcludeDirs: []string{"node_modules"}})
		if err != nil {
			t.Fatal(err)
		}
		if got := repoNames(res.Repos); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("depth %d: got %v, want %v", tt.depth, got, tt.want)
		}
	}
}

func TestFindRepositoriesDoesNotWalkInsideRepo(t *testing.T) {
	setupGitEnv(t)
	base := buildTree(t)

	res, err := DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: 3, ExcludeDirs: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range repoNames(res.Repos) {
		if name == "level1/sub/nested" {
			t.Fatal("nested repository inside level1 must not be found")
		}
	}
	// Visited: base, group, group/org. Never level1 (repository) nor level1/sub.
	if res.VisitedDirs != 3 {
		t.Errorf("VisitedDirs = %d, want 3 (base, group, group/org)", res.VisitedDirs)
	}

	// Maximum depth: base, group, org, team, x and x/y are read (x/y/z is level 6: not read)
	res, err = DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: MaxSearchDepth, ExcludeDirs: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.VisitedDirs != 6 {
		t.Errorf("VisitedDirs with depth %d = %d, want 6", MaxSearchDepth, res.VisitedDirs)
	}
}

func TestFindRepositoriesExcludeDirsHiddenAndSymlinks(t *testing.T) {
	setupGitEnv(t)
	base := buildTree(t)

	res, err := DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: 3, ExcludeDirs: []string{"node_modules", " "}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range repoNames(res.Repos) {
		switch name {
		case "node_modules/pkg", ".hidden/repo", "link/level2":
			t.Errorf("repository %q should not be found", name)
		}
	}
	if !reflect.DeepEqual(res.IgnoredDirs, []string{"node_modules"}) {
		t.Errorf("IgnoredDirs = %v, want [node_modules]", res.IgnoredDirs)
	}

	// Without the exclusion the repository inside node_modules is found
	res, _ = DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: 2})
	found := false
	for _, name := range repoNames(res.Repos) {
		found = found || name == "node_modules/pkg"
	}
	if !found {
		t.Error("node_modules/pkg should be found without ExcludeDirs")
	}
}

func TestFindRepositoriesWorktree(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	main := filepath.Join(t.TempDir(), "main")
	initRepo(t, main, "main")
	runGit(t, main, "worktree", "add", "-q", "-b", "wt", filepath.Join(base, "wt"))

	res, err := DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := repoNames(res.Repos); !reflect.DeepEqual(got, []string{"wt"}) {
		t.Errorf("got %v, want [wt]", got)
	}
}

func TestDiscoveryResultSummary(t *testing.T) {
	setupGitEnv(t)
	base := buildTree(t)

	var calls, lastDirs, lastRepos int
	res, err := DiscoverRepositories(DiscoveryOptions{
		BaseDir:     base,
		MaxDepth:    2,
		ExcludeDirs: []string{"node_modules"},
		OnProgress:  func(dirs, repos int) { calls++; lastDirs, lastRepos = dirs, repos },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.MaxDepth != 2 || res.VisitedDirs != 2 || res.Duration <= 0 {
		t.Errorf("unexpected result: %+v", res)
	}
	if calls != res.VisitedDirs || lastDirs != res.VisitedDirs || lastRepos != len(res.Repos) {
		t.Errorf("OnProgress: calls=%d last=(%d,%d), result=(%d,%d)", calls, lastDirs, lastRepos, res.VisitedDirs, len(res.Repos))
	}
	for _, r := range res.Repos {
		if !r.IsValid || r.Path != filepath.Join(base, filepath.FromSlash(r.Name)) {
			t.Errorf("unexpected repository: %+v", r)
		}
	}
}

func TestDiscoveryPermissionWarning(t *testing.T) {
	setupGitEnv(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	base := t.TempDir()
	initRepo(t, filepath.Join(base, "ok"), "main")
	locked := filepath.Join(base, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	res, err := DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: 2})
	if err != nil {
		t.Fatalf("a subdirectory without permission must not be fatal: %v", err)
	}
	if len(res.Warnings) != 1 || !reflect.DeepEqual(repoNames(res.Repos), []string{"ok"}) {
		t.Errorf("warnings=%v repos=%v", res.Warnings, repoNames(res.Repos))
	}
}

func TestDiscoveryBaseDirErrors(t *testing.T) {
	if _, err := DiscoverRepositories(DiscoveryOptions{BaseDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("missing base directory should fail")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverRepositories(DiscoveryOptions{BaseDir: file}); err == nil {
		t.Error("a file as base directory should fail")
	}
}

func TestRepositoryRelativeName(t *testing.T) {
	setupGitEnv(t)
	base := buildTree(t)
	res, _ := DiscoverRepositories(DiscoveryOptions{BaseDir: base, MaxDepth: 2, ExcludeDirs: []string{"node_modules"}})
	want := map[string]string{
		"level1":       filepath.Join(base, "level1"),
		"group/level2": filepath.Join(base, "group", "level2"),
	}
	for _, r := range res.Repos {
		if want[r.Name] != r.Path {
			t.Errorf("name %q -> path %q", r.Name, r.Path)
		}
	}

	// FindRepositories (compatibility) keeps only the first level and fills the branch
	repos, err := FindRepositories(base)
	if err != nil || len(repos) != 1 || repos[0].Name != "level1" || repos[0].CurrentBranch != "main" {
		t.Errorf("FindRepositories = %+v, %v", repos, err)
	}
}
