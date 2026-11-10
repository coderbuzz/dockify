package app

import "testing"

func TestNormalizeGitRepo(t *testing.T) {
	want := "github.com/amg-id/cmsapi-3"
	for _, in := range []string{
		"https://github.com/amg-id/cmsapi-3",
		"https://github.com/amg-id/cmsapi-3.git",
		"https://github.com/amg-id/cmsapi-3/",
		"  https://GitHub.com/AMG-ID/cmsapi-3.git ",
		"http://github.com/amg-id/cmsapi-3",
		"https://user:token@github.com/amg-id/cmsapi-3.git",
		"git@github.com:amg-id/cmsapi-3.git",
		"ssh://git@github.com/amg-id/cmsapi-3.git",
		"github.com/amg-id/cmsapi-3",
	} {
		if got := normalizeGitRepo(in); got != want {
			t.Errorf("normalizeGitRepo(%q) = %q, want %q", in, got, want)
		}
	}
	if a, b := normalizeGitRepo("https://github.com/amg-id/cmsapi-3"), normalizeGitRepo("https://github.com/amg-id/cmsapi-2"); a == b {
		t.Errorf("different repos normalized equal: %q", a)
	}
}

func TestWebhookDeployable(t *testing.T) {
	cases := map[string]bool{
		StatusRunning:   true,
		StatusFailed:    true,
		StatusStopped:   false,
		StatusDeploying: false,
		StatusCreated:   false,
		StatusDraft:     false,
	}
	for status, want := range cases {
		if got := webhookDeployable(status); got != want {
			t.Errorf("webhookDeployable(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestRepo_FindAllByGitRepoNormalizesURL(t *testing.T) {
	repo, srvRepo := setupRepo(t)
	srvID := createTestServer(t, srvRepo)
	mk := func(name, gitRepo, branch, status string) {
		t.Helper()
		a := &App{Name: name, ServerID: srvID, Compose: "services: {}", GitRepo: gitRepo, GitBranch: branch, Status: status}
		if err := repo.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	mk("no-suffix", "https://github.com/amg-id/cmsapi-3", "staging", StatusFailed)
	mk("with-suffix", "https://github.com/amg-id/cmsapi-3.git", "staging", StatusRunning)
	mk("other-branch", "https://github.com/amg-id/cmsapi-3", "main", StatusRunning)
	mk("other-repo", "https://github.com/amg-id/cmsapi-2.git", "staging", StatusRunning)
	mk("draft", "https://github.com/amg-id/cmsapi-3.git", "staging", StatusDraft)
	mk("no-repo", "", "staging", StatusRunning)

	apps, err := repo.FindAllByGitRepo("https://github.com/amg-id/cmsapi-3.git", "staging")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range apps {
		names = append(names, a.Name)
	}
	if len(names) != 2 || names[0] != "no-suffix" || names[1] != "with-suffix" {
		t.Fatalf("got %v, want [no-suffix with-suffix]", names)
	}
}
