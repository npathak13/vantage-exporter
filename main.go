package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Skill represents a Vantage skill
type Skill struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Parameter represents transaction/file parameters
type Parameter struct {
	IsReadOnly bool   `json:"isReadOnly"`
	Key        string `json:"key"`
	Value      string `json:"value"`
}

// StageDto represents transaction stage information
type StageDto struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// Transaction represents a Vantage transaction with actual API fields
type Transaction struct {
	ID                        string      `json:"transactionId"`
	SkillID                   string      `json:"skillId"`
	SkillVersion              int         `json:"skillVersion"`
	Status                    string      `json:"status"`
	CreateTimeUtc             string      `json:"createTimeUtc"`
	CompletedUtc              string      `json:"completedUtc,omitempty"`
	DocumentCount             int         `json:"documentCount"`
	PageCount                 int         `json:"pageCount"`
	TransactionParameters     []Parameter `json:"transactionParameters"`
	FileParameters            []Parameter `json:"fileParameters"`
	Error                     string      `json:"error,omitempty"`
	Stage                     StageDto    `json:"stage,omitempty"`
	ManualReviewOperatorName  string      `json:"manualReviewOperatorName,omitempty"`
	ManualReviewOperatorEmail string      `json:"manualReviewOperatorEmail,omitempty"`
}

// TransactionResponse represents the API response structure
type TransactionResponse struct {
	Items          []Transaction `json:"items"`
	TotalItemCount int           `json:"totalItemCount"`
}

// TokenResponse represents OAuth2 token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

// VantageCollector implements prometheus.Collector
type vantageCollector struct {
	// Existing metrics
	skillMetric                *prometheus.Desc
	transactionMetric          *prometheus.Desc
	completedTransactionMetric *prometheus.Desc

	// New enhanced metrics
	transactionCreatedMetric      *prometheus.Desc
	transactionPageCountMetric    *prometheus.Desc
	skillVersionMetric            *prometheus.Desc
	transactionFileCountMetric    *prometheus.Desc
	transactionDocumentCountMetric *prometheus.Desc

	baseURL      string
	clientID     string
	clientSecret string
}

// newVantageCollector creates a new collector with all metrics
func newVantageCollector() *vantageCollector {
	return &vantageCollector{
		// Existing metrics
		skillMetric: prometheus.NewDesc(
			"vantage_skill_info",
			"Vantage skill information",
			[]string{"skill_id", "skill_name", "skill_type"}, nil,
		),
		transactionMetric: prometheus.NewDesc(
			"vantage_active_transaction",
			"Vantage active transaction",
			[]string{"transaction_id", "skill_id"}, nil,
		),
		completedTransactionMetric: prometheus.NewDesc(
			"vantage_completed_transactions_total",
			"Total completed transactions by skill and status",
			[]string{"skill_id", "status"}, nil,
		),

		// New enhanced metrics
		transactionCreatedMetric: prometheus.NewDesc(
			"vantage_transaction_created_timestamp",
			"Transaction creation timestamp",
			[]string{"skill_id", "transaction_id"}, nil,
		),
		transactionPageCountMetric: prometheus.NewDesc(
			"vantage_transaction_page_count",
			"Number of pages per transaction",
			[]string{"skill_id", "transaction_id"}, nil,
		),
		skillVersionMetric: prometheus.NewDesc(
			"vantage_skill_version",
			"Skill version used for transaction",
			[]string{"skill_id", "version"}, nil,
		),
		transactionFileCountMetric: prometheus.NewDesc(
			"vantage_transaction_file_count",
			"Number of source files per transaction",
			[]string{"skill_id", "transaction_id"}, nil,
		),
		transactionDocumentCountMetric: prometheus.NewDesc(
			"vantage_transaction_document_count",
			"Number of extracted documents per transaction",
			[]string{"skill_id", "transaction_id"}, nil,
		),

		baseURL:      getEnv("VANTAGE_BASE_URL", "https://vantage-us.abbyy.com"),
		clientID:     os.Getenv("VANTAGE_CLIENT_ID"),
		clientSecret: os.Getenv("VANTAGE_CLIENT_SECRET"),
	}
}

// Describe implements prometheus.Collector
func (c *vantageCollector) Describe(ch chan<- *prometheus.Desc) {
	// Existing metrics
	ch <- c.skillMetric
	ch <- c.transactionMetric
	ch <- c.completedTransactionMetric

	// New enhanced metrics
	ch <- c.transactionCreatedMetric
	ch <- c.transactionPageCountMetric
	ch <- c.skillVersionMetric
	ch <- c.transactionFileCountMetric
	ch <- c.transactionDocumentCountMetric
}

// Collect implements prometheus.Collector
func (c *vantageCollector) Collect(ch chan<- prometheus.Metric) {
	// Existing skill collection
	skills, err := c.getSkills()
	if err != nil {
		log.Printf("Error getting skills: %v", err)
	} else {
		for _, skill := range skills {
			ch <- prometheus.MustNewConstMetric(
				c.skillMetric,
				prometheus.GaugeValue,
				1,
				skill.ID, skill.Name, skill.Type,
			)
		}
	}

	// Enhanced active transactions collection
	activeTransactions, err := c.getActiveTransactions()
	if err != nil {
		log.Printf("Error getting active transactions: %v", err)
	} else {
		log.Printf("Found %d active transactions", len(activeTransactions))

		for _, tx := range activeTransactions {
			// Existing active transaction metric
			ch <- prometheus.MustNewConstMetric(
				c.transactionMetric,
				prometheus.GaugeValue,
				1,
				tx.ID, tx.SkillID,
			)
		}
	}

	// Enhanced completed transactions collection
	completedTransactions, err := c.getCompletedTransactions()
	if err != nil {
		log.Printf("Error getting completed transactions: %v", err)
	} else {
		statusCounts := make(map[string]map[string]int)
		skillVersionsSeen := make(map[string]bool) // Track seen skill+version combinations

		for _, tx := range completedTransactions {
			skillID := tx.SkillID
			status := tx.Status

			if statusCounts[skillID] == nil {
				statusCounts[skillID] = make(map[string]int)
			}
			statusCounts[skillID][status]++

			// Debug: Check timestamp data
			log.Printf("Transaction %s: CreateTime=%s, CompletedTime=%s", tx.ID, tx.CreateTimeUtc, tx.CompletedUtc)

			// Transaction creation timestamp - FIX THE PARSING
			if tx.CreateTimeUtc != "" {
				// API returns format like "2025-08-27T17:43:44.52" without timezone
				var createdTime time.Time
				var err error

				// Try multiple timestamp formats
				formats := []string{
					time.RFC3339,                    // Standard: 2025-08-27T17:43:44Z
					"2006-01-02T15:04:05.999",      // Without timezone: 2025-08-27T17:43:44.52
					"2006-01-02T15:04:05.999999",   // With microseconds
					"2006-01-02T15:04:05",          // Without fractional seconds
				}

				for _, format := range formats {
					createdTime, err = time.Parse(format, tx.CreateTimeUtc)
					if err == nil {
						break
					}
				}

				if err == nil {
					ch <- prometheus.MustNewConstMetric(
						c.transactionCreatedMetric,
						prometheus.GaugeValue,
						float64(createdTime.Unix()),
						tx.SkillID, tx.ID,
					)
					log.Printf("✅ Successfully parsed timestamp for %s: %s -> %d", tx.ID, tx.CreateTimeUtc, createdTime.Unix())
				} else {
					log.Printf("❌ Failed to parse timestamp for %s: %s (error: %v)", tx.ID, tx.CreateTimeUtc, err)
				}
			}

			// Document and page counts from actual API response
			ch <- prometheus.MustNewConstMetric(
				c.transactionDocumentCountMetric,
				prometheus.GaugeValue,
				float64(tx.DocumentCount),
				tx.SkillID, tx.ID,
			)

			// Page count metric
			ch <- prometheus.MustNewConstMetric(
				c.transactionPageCountMetric,
				prometheus.GaugeValue,
				float64(tx.PageCount),
				tx.SkillID, tx.ID,
			)

			// File count from fileParameters (count unique SourceFileName entries)
			fileCount := 0
			seenFiles := make(map[string]bool)
			for _, param := range tx.FileParameters {
				if param.Key == "SourceFileName" && !seenFiles[param.Value] {
					seenFiles[param.Value] = true
					fileCount++
				}
			}
			ch <- prometheus.MustNewConstMetric(
				c.transactionFileCountMetric,
				prometheus.GaugeValue,
				float64(fileCount),
				tx.SkillID, tx.ID,
			)

			// Skill version tracking (avoid duplicates)
			skillVersionKey := fmt.Sprintf("%s-%d", tx.SkillID, tx.SkillVersion)
			if !skillVersionsSeen[skillVersionKey] {
				skillVersionsSeen[skillVersionKey] = true
				ch <- prometheus.MustNewConstMetric(
					c.skillVersionMetric,
					prometheus.GaugeValue,
					1,
					tx.SkillID, fmt.Sprintf("%d", tx.SkillVersion),
				)
			}
		}

		// Emit completed transaction status counts
		for skillID, statuses := range statusCounts {
			for status, count := range statuses {
				ch <- prometheus.MustNewConstMetric(
					c.completedTransactionMetric,
					prometheus.CounterValue,
					float64(count),
					skillID, status,
				)
			}
		}
	}
}

// getToken gets OAuth2 access token
func (c *vantageCollector) getToken() (string, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	data.Set("scope", "global.wildcard openid permissions")

	resp, err := http.Post(
		c.baseURL+"/auth2/connect/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

// getSkills fetches skills from Vantage API
func (c *vantageCollector) getSkills() ([]Skill, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/publicapi/v1/skills", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Skills API Response Status: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	if len(body) == 0 {
		log.Println("Empty response from skills API")
		return []Skill{}, nil
	}

	var skills []Skill
	if err := json.Unmarshal(body, &skills); err != nil {
		return nil, fmt.Errorf("failed to parse skills JSON: %w", err)
	}

	log.Printf("Found %d skills", len(skills))
	return skills, nil
}

// getActiveTransactions fetches active transactions with enhanced data
func (c *vantageCollector) getActiveTransactions() ([]Transaction, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/publicapi/v1/transactions/active?Limit=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Active Transactions API Response Status: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	if len(body) == 0 {
		log.Println("Empty response from active transactions API")
		return []Transaction{}, nil
	}

	var response TransactionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse active transactions JSON: %w", err)
	}

	log.Printf("Found %d active transactions", len(response.Items))
	return response.Items, nil
}

// getCompletedTransactions fetches completed transactions with enhanced data
func (c *vantageCollector) getCompletedTransactions() ([]Transaction, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/publicapi/v1/transactions/completed?Limit=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Completed Transactions API Response Status: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	if len(body) == 0 {
		log.Println("Empty response from completed transactions API")
		return []Transaction{}, nil
	}

	var response TransactionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse completed transactions JSON: %w", err)
	}

	log.Printf("Found %d completed transactions", len(response.Items))
	return response.Items, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	collector := newVantageCollector()
	prometheus.MustRegister(collector)

	http.Handle("/metrics", promhttp.Handler())
	log.Println("Enhanced Vantage exporter running on :8084/metrics")
	log.Fatal(http.ListenAndServe(":8084", nil))
}