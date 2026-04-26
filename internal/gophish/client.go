package gophish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps the Gophish REST API
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendingProfile represents a Gophish SMTP sending profile
type SendingProfile struct {
	ID               int64  `json:"id,omitempty"`
	Name             string `json:"name"`
	Host             string `json:"host"`
	FromAddress      string `json:"from_address"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	IgnoreCertErrors bool   `json:"ignore_cert_errors"`
}

// Campaign represents a Gophish campaign
type Campaign struct {
	ID             int64         `json:"id,omitempty"`
	Name           string        `json:"name"`
	CreatedDate    string        `json:"created_date,omitempty"`
	CompletedDate  string        `json:"completed_date,omitempty"`
	Status         string        `json:"status,omitempty"`
	Results        []Result      `json:"results,omitempty"`
	Timeline       []Event       `json:"timeline,omitempty"`
	SMTP           SendingProfile `json:"smtp"`
	URL            string        `json:"url,omitempty"`
	LaunchDate     string        `json:"launch_date,omitempty"`
	Groups         []Group       `json:"groups,omitempty"`
	Template       Template      `json:"template,omitempty"`
	Page           Page          `json:"page,omitempty"`
}

type Result struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
	Status    string `json:"status"`
	IP        string `json:"ip"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Event struct {
	Email   string `json:"email"`
	Time    string `json:"time"`
	Message string `json:"message"`
	Details string `json:"details"`
}

type Group struct {
	ID      int64    `json:"id,omitempty"`
	Name    string   `json:"name"`
	Targets []Target `json:"targets,omitempty"`
}

type Target struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
}

type Template struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	Subject     string `json:"subject,omitempty"`
	Text        string `json:"text,omitempty"`
	HTML        string `json:"html,omitempty"`
	Attachments []any  `json:"attachments,omitempty"`
}

type Page struct {
	ID                int64  `json:"id,omitempty"`
	Name              string `json:"name"`
	HTML              string `json:"html,omitempty"`
	CaptureCredentials bool  `json:"capture_credentials"`
	CapturePasswords   bool  `json:"capture_passwords"`
	RedirectURL       string `json:"redirect_url,omitempty"`
}

// Ping checks if the Gophish API is reachable
func (c *Client) Ping() error {
	_, err := c.get("/api/campaigns/")
	return err
}

// GetCampaigns returns all campaigns
func (c *Client) GetCampaigns() ([]Campaign, error) {
	data, err := c.get("/api/campaigns/")
	if err != nil {
		return nil, err
	}
	var campaigns []Campaign
	return campaigns, json.Unmarshal(data, &campaigns)
}

// GetCampaign returns a single campaign by ID
func (c *Client) GetCampaign(id int64) (Campaign, error) {
	data, err := c.get(fmt.Sprintf("/api/campaigns/%d", id))
	if err != nil {
		return Campaign{}, err
	}
	var campaign Campaign
	return campaign, json.Unmarshal(data, &campaign)
}

// CreateSendingProfile creates an SMTP sending profile
func (c *Client) CreateSendingProfile(sp SendingProfile) (SendingProfile, error) {
	data, err := c.post("/api/smtp/", sp)
	if err != nil {
		return SendingProfile{}, err
	}
	var result SendingProfile
	return result, json.Unmarshal(data, &result)
}

// GetSendingProfiles returns all sending profiles
func (c *Client) GetSendingProfiles() ([]SendingProfile, error) {
	data, err := c.get("/api/smtp/")
	if err != nil {
		return nil, err
	}
	var profiles []SendingProfile
	return profiles, json.Unmarshal(data, &profiles)
}

// CreateGroup creates a target group
func (c *Client) CreateGroup(g Group) (Group, error) {
	data, err := c.post("/api/groups/", g)
	if err != nil {
		return Group{}, err
	}
	var result Group
	return result, json.Unmarshal(data, &result)
}

// GetGroups returns all groups
func (c *Client) GetGroups() ([]Group, error) {
	data, err := c.get("/api/groups/")
	if err != nil {
		return nil, err
	}
	var groups []Group
	return groups, json.Unmarshal(data, &groups)
}

// CreateTemplate creates an email template
func (c *Client) CreateTemplate(t Template) (Template, error) {
	data, err := c.post("/api/templates/", t)
	if err != nil {
		return Template{}, err
	}
	var result Template
	return result, json.Unmarshal(data, &result)
}

// CreatePage creates a landing page
func (c *Client) CreatePage(p Page) (Page, error) {
	data, err := c.post("/api/pages/", p)
	if err != nil {
		return Page{}, err
	}
	var result Page
	return result, json.Unmarshal(data, &result)
}

// GetTemplates returns all email templates
func (c *Client) GetTemplates() ([]Template, error) {
	data, err := c.get("/api/templates/")
	if err != nil {
		return nil, err
	}
	var templates []Template
	return templates, json.Unmarshal(data, &templates)
}

// GetTemplate returns a single template by ID
func (c *Client) GetTemplate(id int64) (Template, error) {
	data, err := c.get(fmt.Sprintf("/api/templates/%d", id))
	if err != nil {
		return Template{}, err
	}
	var t Template
	return t, json.Unmarshal(data, &t)
}

// DeleteTemplate deletes an email template by ID
func (c *Client) DeleteTemplate(id int64) error {
	return c.delete(fmt.Sprintf("/api/templates/%d", id))
}

// GetPages returns all landing pages
func (c *Client) GetPages() ([]Page, error) {
	data, err := c.get("/api/pages/")
	if err != nil {
		return nil, err
	}
	var pages []Page
	return pages, json.Unmarshal(data, &pages)
}

// DeleteCampaign deletes a campaign by ID
func (c *Client) DeleteCampaign(id int64) error {
	return c.delete(fmt.Sprintf("/api/campaigns/%d", id))
}

// CreateCampaign creates and launches a campaign
func (c *Client) CreateCampaign(camp Campaign) (Campaign, error) {
	data, err := c.post("/api/campaigns/", camp)
	if err != nil {
		return Campaign{}, err
	}
	var result Campaign
	return result, json.Unmarshal(data, &result)
}

func (c *Client) get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	return c.doRequest(req)
}

func (c *Client) post(path string, body any) ([]byte, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doRequest(req)
}

func (c *Client) delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	_, err = c.doRequest(req)
	return err
}

func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gophish API request to %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gophish API %s returned %d: %s", req.URL.Path, resp.StatusCode, string(data))
	}

	return data, nil
}
