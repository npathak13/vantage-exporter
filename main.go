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

// DocumentBusinessRulesErrorDto represents business rule errors
type DocumentBusinessRulesErrorDto struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// ResultFile represents output files from processing
type ResultFile struct {
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	Type     string `json:"type"`
}

// SourceFile represents input files
type SourceFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DocumentDetail represents detailed transaction document information
type DocumentDetail struct {
	ID                  string                          `json:"id"`
	ResultFiles         []ResultFile                    `json:"resultFiles"`
	BusinessRulesErrors []DocumentBusinessRulesErrorDto `json:"businessRulesErrors"`
}

// TransactionDetail represents detailed individual transaction response
type TransactionDetail struct {
	ID          string           `json:"id"`
	Status      string           `json:"status"`
	Documents   []DocumentDetail `json:"documents"`
	SourceFiles []SourceFile     `json:"sourceFiles"`
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

// TransactionMetrics represents detailed metrics for a skill
type TransactionMetrics struct {
	SkillID             string         `json:"skill_id"`
	SkillName           string         `json:"skill_name"`
	TotalTransactions   int            `json:"total_transactions"`
	CompletedSuccess    int            `json:"completed_success"`
	CompletedFailed     int            `json:"completed_failed"`
	ActiveProcessing    int            `json:"active_processing"`
	ActiveManualReview  int            `json:"active_manual_review"`
	AveragePages        float64        `json:"avg_pages_per_transaction"`
	AverageDocuments    float64        `json:"avg_documents_per_transaction"`
	BusinessRulesErrors int            `json:"business_rules_errors_total"`
	StageBreakdown      map[string]int `json:"stage_breakdown"`
	StatusBreakdown     map[string]int `json:"status_breakdown"`
	FileTypeBreakdown   map[string]int `json:"file_type_breakdown"`
}

// TokenResponse represents OAuth2 token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

// VantageCollector implements prometheus.Collector
type vantageCollector struct {
	skillMetric                    *prometheus.Desc
	transactionMetric              *prometheus.Desc
	completedTransactionMetric     *prometheus.Desc
	transactionCreatedMetric       *prometheus.Desc
	transactionPageCountMetric     *prometheus.Desc
	skillVersionMetric             *prometheus.Desc
	transactionFileCountMetric     *prometheus.Desc
	transactionDocumentCountMetric *prometheus.Desc
	businessRulesErrorsMetric      *prometheus.Desc
	resultFileTypesMetric          *prometheus.Desc
	processingSuccessMetric        *prometheus.Desc

	baseURL      string
	clientID     string
	clientSecret string
	port         string

	cachedSkills    []Skill
	skillsCacheTime time.Time
}

func newVantageCollector() *vantageCollector {
	return &vantageCollector{
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
		businessRulesErrorsMetric: prometheus.NewDesc(
			"vantage_business_rules_errors_total",
			"Business rule validation errors per transaction",
			[]string{"skill_id", "transaction_id", "error_type"}, nil,
		),
		resultFileTypesMetric: prometheus.NewDesc(
			"vantage_result_file_types_total",
			"Types of result files generated per transaction",
			[]string{"skill_id", "transaction_id", "file_type"}, nil,
		),
		processingSuccessMetric: prometheus.NewDesc(
			"vantage_processing_success",
			"Transaction processing success indicator",
			[]string{"skill_id", "transaction_id", "status"}, nil,
		),

		baseURL:      getEnv("VANTAGE_BASE_URL", "https://vantage-us.abbyy.com"),
		clientID:     getEnv("VANTAGE_CLIENT_ID", ""),
		clientSecret: getEnv("VANTAGE_CLIENT_SECRET", ""),
		port:         getEnv("VANTAGE_METRICS_PORT", "8080"),
	}
}

func (c *vantageCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.skillMetric
	ch <- c.transactionMetric
	ch <- c.completedTransactionMetric
	ch <- c.transactionCreatedMetric
	ch <- c.transactionPageCountMetric
	ch <- c.skillVersionMetric
	ch <- c.transactionFileCountMetric
	ch <- c.transactionDocumentCountMetric
	ch <- c.businessRulesErrorsMetric
	ch <- c.resultFileTypesMetric
	ch <- c.processingSuccessMetric
}

func (c *vantageCollector) Collect(ch chan<- prometheus.Metric) {
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

	activeTransactions, err := c.getActiveTransactions()
	if err != nil {
		log.Printf("Error getting active transactions: %v", err)
	} else {
		log.Printf("Found %d active transactions", len(activeTransactions))

		for _, tx := range activeTransactions {
			ch <- prometheus.MustNewConstMetric(
				c.transactionMetric,
				prometheus.GaugeValue,
				1,
				tx.ID, tx.SkillID,
			)
		}
	}

	completedTransactions, err := c.getCompletedTransactions()
	if err != nil {
		log.Printf("Error getting completed transactions: %v", err)
	} else {
		statusCounts := make(map[string]map[string]int)
		skillVersionsSeen := make(map[string]bool)

		for _, tx := range completedTransactions {
			skillID := tx.SkillID
			status := tx.Status

			if statusCounts[skillID] == nil {
				statusCounts[skillID] = make(map[string]int)
			}
			statusCounts[skillID][status]++

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

// getActiveTransactions fetches active transactions from Vantage API
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

// getTransactionDetail fetches detailed information for a single transaction
func (c *vantageCollector) getTransactionDetail(transactionID string) (*TransactionDetail, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/publicapi/v1/transactions/"+transactionID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var detail TransactionDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("failed to parse transaction detail JSON: %w", err)
	}

	return &detail, nil
}

// handleTransactionDetails handles the multi-skill transaction details endpoint
func (c *vantageCollector) handleTransactionDetails(w http.ResponseWriter, r *http.Request) {
	// Parse skills parameter
	skillsParam := r.URL.Query().Get("skills")
	if skillsParam == "" {
		http.Error(w, "skills parameter required (e.g., ?skills=skill1,skill2,skill3)", http.StatusBadRequest)
		return
	}

	// Parse comma-separated skill IDs (handle Grafana format with braces)
	skillsParam = strings.Trim(skillsParam, "{}")
	skillIds := strings.Split(skillsParam, ",")

	// Clean up skill IDs
	for i := range skillIds {
		skillIds[i] = strings.TrimSpace(skillIds[i])
	}

	if len(skillIds) == 0 || (len(skillIds) == 1 && skillIds[0] == "") {
		http.Error(w, "no valid skill IDs provided", http.StatusBadRequest)
		return
	}

	log.Printf("Processing transaction details for %d skills: %v", len(skillIds), skillIds)

	// Get fresh data using your existing methods
	skills, err := c.getSkills()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get skills: %v", err), http.StatusInternalServerError)
		return
	}

	activeTransactions, err := c.getActiveTransactions()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get active transactions: %v", err), http.StatusInternalServerError)
		return
	}

	completedTransactions, err := c.getCompletedTransactions()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get completed transactions: %v", err), http.StatusInternalServerError)
		return
	}

	// Create skill name lookup
	skillNames := make(map[string]string)
	for _, skill := range skills {
		skillNames[skill.ID] = skill.Name
	}

	// Process each requested skill
	var results []TransactionMetrics

	for _, skillId := range skillIds {
		if skillId == "" {
			continue
		}

		skillName := skillNames[skillId]
		if skillName == "" {
			skillName = skillId // fallback
		}

		metrics := TransactionMetrics{
			SkillID:           skillId,
			SkillName:         skillName,
			StageBreakdown:    make(map[string]int),
			StatusBreakdown:   make(map[string]int),
			FileTypeBreakdown: make(map[string]int),
		}

		// Process active transactions for this skill
		var totalPages, totalDocs int
		for _, tx := range activeTransactions {
			if tx.SkillID != skillId {
				continue
			}

			metrics.TotalTransactions++
			totalPages += tx.PageCount
			totalDocs += tx.DocumentCount

			// Stage breakdown
			if tx.Stage.Name != "" {
				metrics.StageBreakdown[tx.Stage.Name]++
			}
			if tx.Stage.Type != "" {
				metrics.StageBreakdown[tx.Stage.Type]++
			}

			// Count manual review vs processing
			if tx.ManualReviewOperatorName != "" || tx.ManualReviewOperatorEmail != "" {
				metrics.ActiveManualReview++
			} else {
				metrics.ActiveProcessing++
			}
		}

		// Process completed transactions for this skill
		for _, tx := range completedTransactions {
			if tx.SkillID != skillId {
				continue
			}

			metrics.TotalTransactions++
			totalPages += tx.PageCount
			totalDocs += tx.DocumentCount

			// Status breakdown
			metrics.StatusBreakdown[tx.Status]++

			if tx.Status == "Finished Successfully" {
				metrics.CompletedSuccess++
			} else if tx.Status == "Failed" {
				metrics.CompletedFailed++
			}
		}

		// Calculate averages
		if metrics.TotalTransactions > 0 {
			metrics.AveragePages = float64(totalPages) / float64(metrics.TotalTransactions)
			metrics.AverageDocuments = float64(totalDocs) / float64(metrics.TotalTransactions)
		}

		results = append(results, metrics)
		log.Printf("Processed skill %s (%s): %d total transactions", skillId, skillName, metrics.TotalTransactions)
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully returned metrics for %d skills", len(results))
}

func (c *vantageCollector) handleSkillsList(w http.ResponseWriter, r *http.Request) {
	if time.Since(c.skillsCacheTime) < 5*time.Minute && len(c.cachedSkills) > 0 {
		log.Printf("Using cached skills (%d skills)", len(c.cachedSkills))
	} else {
		skills, err := c.getSkills()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get skills: %v", err), http.StatusInternalServerError)
			return
		}
		c.cachedSkills = skills
		c.skillsCacheTime = time.Now()
		log.Printf("Refreshed skills cache (%d skills)", len(skills))
	}

	type SkillOption struct {
		Value string `json:"value"`
		Text  string `json:"text"`
	}

	var options []SkillOption
	for _, skill := range c.cachedSkills {
		options = append(options, SkillOption{
			Value: skill.ID,
			Text:  fmt.Sprintf("%s (%s)", skill.Name, skill.ID),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(options); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Returned %d skills for template variables", len(options))
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
	http.HandleFunc("/transaction-details", collector.handleTransactionDetails)
	http.HandleFunc("/skills", collector.handleSkillsList)

	log.Printf("Vantage exporter running on :%s", collector.port)
	log.Println("Endpoints:")
	log.Println("  /metrics - Prometheus metrics")
	log.Println("  /transaction-details?skills=skill1,skill2,skill3 - Multi-skill transaction details")
	log.Println("  /skills - Skills list for Grafana template variables")

	log.Fatal(http.ListenAndServe(":"+collector.port, nil))
}