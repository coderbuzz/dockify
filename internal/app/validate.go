package app

import (
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// User strings that end up in worker shell commands, file paths or the .env
// file are checked here, at the trust boundary (API, web forms, import).
var (
	domainRe   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
	envKeyRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	filePathRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// ValidDomain returns d lowercased and trimmed if it is a plain hostname.
func ValidDomain(d string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(d))
	if !domainRe.MatchString(n) {
		return "", fmt.Errorf("invalid domain %q", d)
	}
	return n, nil
}

// ValidFilePath returns p cleaned if it is a relative path inside the app
// folder made of [A-Za-z0-9._/-].
func ValidFilePath(p string) (string, error) {
	c := path.Clean(strings.TrimSpace(p))
	if c == "." || strings.HasPrefix(c, "/") || c == ".." || strings.HasPrefix(c, "../") || !filePathRe.MatchString(c) {
		return "", fmt.Errorf("invalid file path %q", p)
	}
	return c, nil
}

func ValidEnvKey(k string) error {
	if !envKeyRe.MatchString(k) {
		return fmt.Errorf("invalid env key %q", k)
	}
	return nil
}

// validateForm checks the domains, env keys and file paths of the add/edit form.
func validateForm(r *http.Request, domains []string) error {
	for _, d := range domains {
		if d == "" {
			continue
		}
		if _, err := ValidDomain(d); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, k := range r.Form["env_key"] {
		if k = strings.TrimSpace(k); k != "" {
			if err := ValidEnvKey(k); err != nil {
				return err
			}
			// Two rows with one key would overwrite each other (and a masked secret silently).
			if seen[k] {
				return fmt.Errorf("environment variable %s is listed twice", k)
			}
			seen[k] = true
		}
	}
	for _, p := range r.Form["file_path"] {
		if strings.TrimSpace(p) != "" {
			if _, err := ValidFilePath(p); err != nil {
				return err
			}
		}
	}
	return nil
}

var envEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "$", "$$")

// envFile renders the app's .env for docker compose: one KEY="value" line per
// variable. Values are trimmed, as compose trimmed the unquoted values written
// before, so apps keep receiving the same bytes.
func envFile(secrets []AppSecret) string {
	var b strings.Builder
	for _, s := range secrets {
		fmt.Fprintf(&b, "%s=\"%s\"\n", s.Key, envEscaper.Replace(strings.TrimSpace(s.Value)))
	}
	return b.String()
}
