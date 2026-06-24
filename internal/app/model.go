package app

import "time"

type App struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	ServerID  int64     `json:"server_id"`
	Domain    string    `json:"domain"`
	Port      int       `json:"port"`
	Compose   string    `json:"compose"`
	GitRepo   string    `json:"git_repo"`
	GitBranch string    `json:"git_branch"`
	AuthUser  string    `json:"auth_user"`
	AuthPass  string    `json:"auth_pass"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Deployment struct {
	ID              int64     `json:"id"`
	AppID           int64     `json:"app_id"`
	ServerID        int64     `json:"server_id"`
	Status          string    `json:"status"`
	Log             string    `json:"log"`
	CommitSHA       string    `json:"commit_sha"`
	ComposeSnapshot string    `json:"compose_snapshot"`
	CreatedAt       time.Time `json:"created_at"`
}
