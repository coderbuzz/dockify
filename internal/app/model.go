package app

import "time"

type App struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	ServerID          int64     `json:"server_id"`
	Domain            string    `json:"domain"`
	Port              int       `json:"port"`
	Compose           string    `json:"compose"`
	GitRepo           string    `json:"git_repo"`
	GitBranch         string    `json:"git_branch"`
	AuthUser          string    `json:"auth_user"`
	AuthPass          string    `json:"auth_pass"`
	Status            string    `json:"status"`
	ComposeMode       string    `json:"compose_mode"`
	MemoryLimit       string    `json:"memory_limit"`
	CPULimit          string    `json:"cpu_limit"`
	LogMaxSize        string    `json:"log_max_size"`
	LogMaxFile        string    `json:"log_max_file"`
	Command           string    `json:"command"`
	Ports             string    `json:"ports"`
	UlimitsNofile     string    `json:"ulimits_nofile"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
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

type ContainerStats struct {
	ID             int64     `json:"id"`
	AppID          int64     `json:"app_id"`
	ServerID       int64     `json:"server_id"`
	ContainerName  string    `json:"container_name"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemUsageBytes  int64     `json:"mem_usage_bytes"`
	MemLimitBytes  int64     `json:"mem_limit_bytes"`
	MemPercent     float64   `json:"mem_percent"`
	NetIORxBytes   int64     `json:"net_io_rx_bytes"`
	NetIOTxBytes   int64     `json:"net_io_tx_bytes"`
	BlockIORead    int64     `json:"block_io_read"`
	BlockIOWrite   int64     `json:"block_io_write"`
	PIDs           int       `json:"pids"`
	CreatedAt      time.Time `json:"created_at"`
}

type RouteStats struct {
	ID            int64     `json:"id"`
	AppID         int64     `json:"app_id"`
	Domain        string    `json:"domain"`
	TotalRequests int64     `json:"total_requests"`
	RequestsRPS   float64   `json:"requests_rps"`
	Status2xx     int64     `json:"status_2xx"`
	Status3xx     int64     `json:"status_3xx"`
	Status4xx     int64     `json:"status_4xx"`
	Status5xx     int64     `json:"status_5xx"`
	CreatedAt     time.Time `json:"created_at"`
}

type ChartPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

type StatsOverview struct {
	CPUPercent     float64 `json:"cpu_percent"`
	MemPercent     float64 `json:"mem_percent"`
	MemUsageBytes  int64   `json:"mem_usage_bytes"`
	MemLimitBytes  int64   `json:"mem_limit_bytes"`
	NetIORxBytes   int64   `json:"net_io_rx_bytes"`
	NetIOTxBytes   int64   `json:"net_io_tx_bytes"`
	BlockIORead    int64   `json:"block_io_read"`
	BlockIOWrite   int64   `json:"block_io_write"`
	PIDs           int     `json:"pids"`
	ContainerName  string  `json:"container_name"`
}

type TrafficOverview struct {
	RequestsRPS   float64 `json:"requests_rps"`
	TotalRequests int64   `json:"total_requests"`
	Status2xx     int64   `json:"status_2xx"`
	Status3xx     int64   `json:"status_3xx"`
	Status4xx     int64   `json:"status_4xx"`
	Status5xx     int64   `json:"status_5xx"`
}
