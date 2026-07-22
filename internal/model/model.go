package model

type AppSummary struct {
	ID        int64
	Name      string
	Domain    string
	Port      int
	Status    string
	GitRepo   string
	GitBranch string
}

type ChartPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}
