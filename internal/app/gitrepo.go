package app

import "strings"

// normalizeGitRepo reduces a git remote URL to "host/owner/repo" so webhook
// payloads match the URL stored on an app regardless of scheme, credentials,
// a trailing ".git" or slash, or letter case. GitHub sends clone_url with
// ".git" while users often paste the browser URL without it.
//
//	https://github.com/Org/Repo.git  -> github.com/org/repo
//	git@github.com:org/repo.git      -> github.com/org/repo
//	ssh://git@github.com/org/repo/   -> github.com/org/repo
func normalizeGitRepo(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	} else if at := strings.Index(s, "@"); at >= 0 {
		// scp-like syntax: user@host:path
		if colon := strings.Index(s[at:], ":"); colon >= 0 {
			s = s[:at+colon] + "/" + s[at+colon+1:]
		}
	}
	if slash := strings.Index(s, "/"); slash >= 0 {
		if at := strings.LastIndex(s[:slash], "@"); at >= 0 {
			s = s[at+1:]
		}
	}
	s = strings.TrimRight(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return strings.TrimRight(s, "/")
}

// webhookDeployable reports whether a push webhook may redeploy an app in the
// given status. Failed apps are included so a deploy that failed before its
// image existed recovers on the next push; stopped apps stay stopped.
func webhookDeployable(status string) bool {
	return status == StatusRunning || status == StatusFailed
}
