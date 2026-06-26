package server

import "time"

type Server struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	User      string    `json:"user"`
	SSHKey    string    `json:"ssh_key"`
	Status    string    `json:"status"`
	CPUCores  int       `json:"cpu_cores"`
	RAMMB     int       `json:"ram_mb"`
	DiskGB    int       `json:"disk_gb"`
	CPUUsage  float64   `json:"cpu_usage"`
	RAMUsage  float64   `json:"ram_usage"`
	DiskUsage         float64   `json:"disk_usage"`
	ResourcesUpdatedAt time.Time `json:"resources_updated_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
