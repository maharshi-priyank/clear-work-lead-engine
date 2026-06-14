package campaign

import "time"

type Status string

const (
	StatusPending Status = "PENDING"
	StatusRunning Status = "RUNNING"
	StatusDone    Status = "DONE"
	StatusFailed  Status = "FAILED"
)

type Filters struct {
	Niche       string `json:"niche,omitempty"`
	Location    string `json:"location,omitempty"`
	JobTitle    string `json:"jobTitle,omitempty"`
	CompanySize string `json:"companySize,omitempty"`
	Keyword     string `json:"keyword,omitempty"`
	TargetCount int    `json:"targetCount,omitempty"`
}

type Campaign struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId"`
	Name          string    `json:"name"`
	Filters       Filters   `json:"filters"`
	Providers     []string  `json:"providers"`
	Status        Status    `json:"status"`
	TotalFound    int       `json:"totalFound"`
	TotalImported int       `json:"totalImported"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type CreateDTO struct {
	Name      string   `json:"name"`
	Providers []string `json:"providers"`
	Filters   *Filters `json:"filters,omitempty"`
}
