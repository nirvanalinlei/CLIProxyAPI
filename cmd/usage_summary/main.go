package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type usageSummaryFile struct {
	GeneratedAt          time.Time `json:"generated_at"`
	RequestTimestamp     time.Time `json:"request_timestamp"`
	APIResponseTimestamp time.Time `json:"api_response_timestamp"`
	LogFile              string    `json:"log_file"`
	StatusCode           int       `json:"status_code"`
	Provider             string    `json:"provider"`
	AuthID               string    `json:"auth_id"`
	AuthLabel            string    `json:"auth_label"`
	AuthType             string    `json:"auth_type"`
	Model                string    `json:"model"`
	PlanType             string    `json:"plan_type"`
	ActiveLimit          string    `json:"active_limit"`
	UsageFound           bool      `json:"usage_found"`
	UsageSource          string    `json:"usage_source"`
	InputTokens          *int64    `json:"input_tokens"`
	OutputTokens         *int64    `json:"output_tokens"`
	TotalTokens          *int64    `json:"total_tokens"`
	CachedInputTokens    *int64    `json:"cached_input_tokens"`
	ReasoningTokens      *int64    `json:"reasoning_tokens"`
}

type aggregateKey struct {
	Provider  string
	AuthID    string
	AuthLabel string
	PlanType  string
	Model     string
}

type aggregateRow struct {
	Key               aggregateKey
	RequestCount      int64
	UsageFoundCount   int64
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	CachedInputTokens int64
	ReasoningTokens   int64
	FirstRequest      time.Time
	LastRequest       time.Time
}

func main() {
	logsDir := flag.String("logs-dir", "logs", "Directory containing *.usage.json files")
	provider := flag.String("provider", "", "Filter by provider")
	authID := flag.String("auth-id", "", "Filter by auth_id")
	planType := flag.String("plan-type", "", "Filter by plan_type")
	model := flag.String("model", "", "Filter by model")
	since := flag.String("since", "", "Include records at or after RFC3339 timestamp")
	jsonOut := flag.Bool("json", false, "Emit JSON instead of text")
	flag.Parse()

	sinceTime, err := parseOptionalTime(*since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --since: %v\n", err)
		os.Exit(1)
	}

	entries, err := filepath.Glob(filepath.Join(*logsDir, "*.usage.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "glob usage files: %v\n", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "no usage summary files found in %s\n", *logsDir)
		os.Exit(1)
	}

	rows := make(map[aggregateKey]*aggregateRow)
	var fileCount int
	for _, path := range entries {
		record, err := loadUsageSummary(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", path, err)
			continue
		}
		if !matchFilters(record, *provider, *authID, *planType, *model, sinceTime) {
			continue
		}
		fileCount++
		resolvedPlanType := fallback(record.PlanType, inferPlanType(record.AuthID, record.AuthLabel, record.ActiveLimit))
		key := aggregateKey{
			Provider:  fallback(record.Provider, "unknown"),
			AuthID:    fallback(record.AuthID, "unknown"),
			AuthLabel: fallback(record.AuthLabel, "unknown"),
			PlanType:  fallback(resolvedPlanType, "unknown"),
			Model:     fallback(record.Model, "unknown"),
		}
		row := rows[key]
		if row == nil {
			row = &aggregateRow{Key: key, FirstRequest: recordTime(record), LastRequest: recordTime(record)}
			rows[key] = row
		}
		row.RequestCount++
		if record.UsageFound {
			row.UsageFoundCount++
		}
		row.InputTokens += ptrValue(record.InputTokens)
		row.OutputTokens += ptrValue(record.OutputTokens)
		row.TotalTokens += ptrValue(record.TotalTokens)
		row.CachedInputTokens += ptrValue(record.CachedInputTokens)
		row.ReasoningTokens += ptrValue(record.ReasoningTokens)
		timestamp := recordTime(record)
		if !timestamp.IsZero() && (row.FirstRequest.IsZero() || timestamp.Before(row.FirstRequest)) {
			row.FirstRequest = timestamp
		}
		if !timestamp.IsZero() && (row.LastRequest.IsZero() || timestamp.After(row.LastRequest)) {
			row.LastRequest = timestamp
		}
	}

	list := make([]aggregateRow, 0, len(rows))
	for _, row := range rows {
		list = append(list, *row)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Key.Provider != list[j].Key.Provider {
			return list[i].Key.Provider < list[j].Key.Provider
		}
		if list[i].Key.AuthID != list[j].Key.AuthID {
			return list[i].Key.AuthID < list[j].Key.AuthID
		}
		if list[i].Key.PlanType != list[j].Key.PlanType {
			return list[i].Key.PlanType < list[j].Key.PlanType
		}
		if list[i].Key.Model != list[j].Key.Model {
			return list[i].Key.Model < list[j].Key.Model
		}
		return list[i].Key.AuthLabel < list[j].Key.AuthLabel
	})

	if *jsonOut {
		payload := struct {
			LogsDir   string         `json:"logs_dir"`
			FileCount int            `json:"file_count"`
			Rows      []aggregateRow `json:"rows"`
		}{
			LogsDir:   *logsDir,
			FileCount: fileCount,
			Rows:      list,
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("logs_dir=%s\n", *logsDir)
	fmt.Printf("matched_files=%d\n", fileCount)
	for _, row := range list {
		fmt.Printf(
			"provider=%s auth_id=%s label=%q plan=%s model=%s requests=%d usage_found=%d input=%d output=%d total=%d cached=%d reasoning=%d first=%s last=%s\n",
			row.Key.Provider,
			row.Key.AuthID,
			row.Key.AuthLabel,
			row.Key.PlanType,
			row.Key.Model,
			row.RequestCount,
			row.UsageFoundCount,
			row.InputTokens,
			row.OutputTokens,
			row.TotalTokens,
			row.CachedInputTokens,
			row.ReasoningTokens,
			formatTime(row.FirstRequest),
			formatTime(row.LastRequest),
		)
	}
}

func loadUsageSummary(path string) (usageSummaryFile, error) {
	var record usageSummaryFile
	content, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(content, &record); err != nil {
		return record, err
	}
	if strings.TrimSpace(record.LogFile) == "" {
		record.LogFile = filepath.Base(path)
	}
	return record, nil
}

func matchFilters(record usageSummaryFile, provider, authID, planType, model string, since time.Time) bool {
	if provider != "" && !strings.EqualFold(record.Provider, provider) {
		return false
	}
	if authID != "" && !strings.EqualFold(record.AuthID, authID) {
		return false
	}
	if planType != "" && !strings.EqualFold(record.PlanType, planType) {
		return false
	}
	if model != "" && !strings.EqualFold(record.Model, model) {
		return false
	}
	if !since.IsZero() {
		timestamp := recordTime(record)
		if timestamp.IsZero() || timestamp.Before(since) {
			return false
		}
	}
	return true
}

func recordTime(record usageSummaryFile) time.Time {
	if !record.RequestTimestamp.IsZero() {
		return record.RequestTimestamp
	}
	if !record.APIResponseTimestamp.IsZero() {
		return record.APIResponseTimestamp
	}
	return record.GeneratedAt
}

func parseOptionalTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

func inferPlanType(authID, authLabel, activeLimit string) string {
	candidate := strings.ToLower(strings.Join([]string{authID, authLabel, activeLimit}, " "))
	switch {
	case strings.Contains(candidate, "plus"):
		return "plus"
	case strings.Contains(candidate, "pro"):
		return "pro"
	case strings.Contains(candidate, "team"):
		return "team"
	case strings.Contains(candidate, "business"):
		return "business"
	case strings.Contains(candidate, "free"):
		return "free"
	default:
		return ""
	}
}

func ptrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func fallback(value, def string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return def
	}
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
