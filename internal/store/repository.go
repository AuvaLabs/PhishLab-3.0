package store

// Repository defines the data access interface for the application
type Repository interface {
	// Engagements
	UpsertEngagement(e Engagement) error
	GetEngagement(id string) (*Engagement, error)
	GetActiveEngagement() (*Engagement, error)

	// Credentials
	InsertCredential(c CapturedCredential) error
	GetCredentials(engagementID string) ([]CapturedCredential, error)
	GetAllCredentials() ([]CapturedCredential, error)
	CredentialCount(engagementID string) (int, error)

	// Campaigns
	InsertCampaign(c CampaignRecord) (int64, error)
	UpdateCampaignStatus(id int64, status string) error
	GetCampaigns(engagementID string) ([]CampaignRecord, error)
	GetCampaignByGophishID(gophishID int64) (*CampaignRecord, error)

	// Timeline events
	InsertTimelineEvent(e TimelineEvent) error
	GetTimeline(engagementID string, limit int) ([]TimelineEvent, error)
	GetTimelineByCampaign(campaignID int64, limit int) ([]TimelineEvent, error)

	Close() error
}

// Ensure DB implements Repository at compile time
var _ Repository = (*DB)(nil)
