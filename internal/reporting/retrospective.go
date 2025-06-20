// Package reporting provides tools for generating reports from historical data.
// The RetrospectiveReporter, for example, analyzes log entries to produce
// summaries of payment activities, error rates, and provider performance
// over a period.
// Refer to spec2_1.md, section 9.3 for more on RetrospectiveReporter.
package reporting

import (
	"time"
)

// LogEntry represents a single log event from the payment orchestration process.
type LogEntry struct {
	Timestamp    time.Time
	RequestID    string
	MerchantID   string
	Status       string // e.g., "SUCCESS", "FAILURE", "RETRY"
	Amount       int64
	Currency     string
	Provider     string // Payment provider used, e.g., "Stripe", "MockAdapter"
	ErrorCode    string // Specific error code, if any
	ErrorMessage string // Detailed error message, if any
}

// RetrospectiveReport summarizes payment activities based on a collection of log entries.
type RetrospectiveReport struct {
	TotalRequests        int
	SuccessfulPayments   int
	FailedPayments       int
	RetriedAttempts      int              // Counts "RETRY" statuses
	TotalAmountProcessed int64            // Sum of amounts for SUCCESSFUL payments only, by default
	AmountByCurrency     map[string]int64 // Sum of amounts for SUCCESSFUL payments, broken down by currency
	ErrorBreakdown       map[string]int   // Count of each ErrorCode for FAILURE statuses
	ProviderUsage        map[string]int   // Count of how many times each provider was attempted (could be for SUCCESS, FAILURE, or RETRY)
	DateFrom             time.Time
	DateTo               time.Time
	ProcessingDuration   time.Duration // Total duration covered by the logs
}

// RetrospectiveReporter generates retrospective reports from log entries.
type RetrospectiveReporter struct{}

// NewRetrospectiveReporter creates a new RetrospectiveReporter.
func NewRetrospectiveReporter() *RetrospectiveReporter {
	return &RetrospectiveReporter{}
}

// GenerateRetrospective analyzes a slice of LogEntry items and produces a RetrospectiveReport.
func (rr *RetrospectiveReporter) GenerateRetrospective(logs []LogEntry) (*RetrospectiveReport, error) {
	if len(logs) == 0 {
		return &RetrospectiveReport{
			AmountByCurrency: make(map[string]int64),
			ErrorBreakdown:   make(map[string]int),
			ProviderUsage:    make(map[string]int),
		}, nil
	}

	report := &RetrospectiveReport{
		AmountByCurrency: make(map[string]int64),
		ErrorBreakdown:   make(map[string]int),
		ProviderUsage:    make(map[string]int),
		DateFrom:         logs[0].Timestamp, // Initialize with the first log's timestamp
		DateTo:           logs[0].Timestamp, // Initialize with the first log's timestamp
	}

	firstTimestampSet := false
	for _, log := range logs {
		report.TotalRequests++ // Assuming each log entry is a request/attempt part of a request flow

		if !firstTimestampSet || log.Timestamp.Before(report.DateFrom) {
			report.DateFrom = log.Timestamp
			// Ensure DateTo is also initialized if this is the very first log processed effectively
			if !firstTimestampSet {
				report.DateTo = log.Timestamp
			}
			firstTimestampSet = true
		}
		if log.Timestamp.After(report.DateTo) {
			report.DateTo = log.Timestamp
		}

		if log.Provider != "" {
			report.ProviderUsage[log.Provider]++
		}

		switch log.Status {
		case "SUCCESS":
			report.SuccessfulPayments++
			report.TotalAmountProcessed += log.Amount
			report.AmountByCurrency[log.Currency] += log.Amount
		case "FAILURE":
			report.FailedPayments++
			if log.ErrorCode != "" {
				report.ErrorBreakdown[log.ErrorCode]++
			}
		case "RETRY":
			report.RetriedAttempts++
		}
	}

	// Calculate ProcessingDuration only if DateFrom and DateTo were meaningfully set
	if firstTimestampSet { // Or check !report.DateFrom.IsZero() if logs could have zero timestamps
		report.ProcessingDuration = report.DateTo.Sub(report.DateFrom)
	}
    // Removed empty else-if branch SA9003


	return report, nil
}
