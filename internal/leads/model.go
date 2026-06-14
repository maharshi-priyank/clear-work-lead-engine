package leads

import "time"

type DiscoveredLead struct {
	ID               string    `json:"id"`
	CampaignID       string    `json:"campaignId"`
	UserID           string    `json:"userId"`
	Source           string    `json:"source"`
	BusinessName     string    `json:"businessName,omitempty"`
	Website          string    `json:"website,omitempty"`
	Email            string    `json:"email,omitempty"`
	Phone            string    `json:"phone,omitempty"`
	ContactName      string    `json:"contactName,omitempty"`
	ContactTitle     string    `json:"contactTitle,omitempty"`
	CompanySize      string    `json:"companySize,omitempty"`
	Location         string    `json:"location,omitempty"`
	LinkedinURL      string    `json:"linkedinUrl,omitempty"`
	TechStack        []string  `json:"techStack"`
	IntentSignals    []string  `json:"intentSignals"`
	IntentScore      int       `json:"intentScore"`
	Description      string    `json:"description,omitempty"`
	Industry         string    `json:"industry,omitempty"`
	ImportedAsLeadID string    `json:"importedAsLeadId,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

type QueryParams struct {
	CampaignID string
	Source     string
	Search     string
	MinScore   int
	Imported   string // "true" | "false" | ""
	Page       int
	Limit      int
}

type PagedResult struct {
	Items []DiscoveredLead `json:"items"`
	Total int              `json:"total"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
}
