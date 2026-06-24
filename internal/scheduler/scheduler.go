package scheduler

import (
	"fmt"
	"math"

	"github.com/coderbuzz/dockify/internal/server"
)

type Scheduler struct {
	serverRepo *server.Repository
}

func New(serverRepo *server.Repository) *Scheduler {
	return &Scheduler{serverRepo: serverRepo}
}

func (s *Scheduler) PickServer() (*server.Server, error) {
	servers, err := s.serverRepo.ListOnline()
	if err != nil {
		return nil, fmt.Errorf("list online servers: %w", err)
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no online servers available")
	}

	var best *server.Server
	bestScore := math.MaxFloat64

	for i := range servers {
		score := (servers[i].CPUUsage * 0.5) + (servers[i].RAMUsage * 0.5)
		if score < bestScore {
			bestScore = score
			best = &servers[i]
		}
	}

	return best, nil
}
