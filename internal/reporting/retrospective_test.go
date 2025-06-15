package reporting

import (
	"reflect"
	"testing"
	"time"
)

func TestRetrospectiveReporter_GenerateRetrospective(t *testing.T) {
	reporter := NewRetrospectiveReporter()

	// Define common timestamps for clarity
	time1 := time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)
	time2 := time.Date(2023, 1, 1, 10, 5, 0, 0, time.UTC)
	time3 := time.Date(2023, 1, 1, 10, 10, 0, 0, time.UTC)
	time4 := time.Date(2023, 1, 1, 9, 55, 0, 0, time.UTC) // Earlier time

	tests := []struct {
		name        string
		logs        []LogEntry
		expected    *RetrospectiveReport
		expectError bool
	}{
		{
			name: "EmptyLogs",
			logs: []LogEntry{},
			expected: &RetrospectiveReport{
				TotalRequests:        0,
				SuccessfulPayments:   0,
				FailedPayments:       0,
				RetriedAttempts:      0,
				TotalAmountProcessed: 0,
				AmountByCurrency:     make(map[string]int64),
				ErrorBreakdown:       make(map[string]int),
				ProviderUsage:        make(map[string]int),
				DateFrom:             time.Time{}, // Zero value for time
				DateTo:               time.Time{}, // Zero value for time
				ProcessingDuration:   0,
			},
		},
		{
			name: "SingleSuccessfulLog",
			logs: []LogEntry{
				{Timestamp: time1, RequestID: "req1", MerchantID: "merch1", Status: "SUCCESS", Amount: 1000, Currency: "USD", Provider: "Stripe"},
			},
			expected: &RetrospectiveReport{
				TotalRequests:        1,
				SuccessfulPayments:   1,
				FailedPayments:       0,
				RetriedAttempts:      0,
				TotalAmountProcessed: 1000,
				AmountByCurrency:     map[string]int64{"USD": 1000},
				ErrorBreakdown:       make(map[string]int),
				ProviderUsage:        map[string]int{"Stripe": 1},
				DateFrom:             time1,
				DateTo:               time1,
				ProcessingDuration:   time1.Sub(time1), // 0
			},
		},
		{
			name: "MixedLogs",
			logs: []LogEntry{
				{Timestamp: time4, RequestID: "req0", MerchantID: "merch1", Status: "SUCCESS", Amount: 500, Currency: "EUR", Provider: "PayPal"}, // Earliest
				{Timestamp: time1, RequestID: "req1", MerchantID: "merch1", Status: "SUCCESS", Amount: 1000, Currency: "USD", Provider: "Stripe"},
				{Timestamp: time2, RequestID: "req2", MerchantID: "merch2", Status: "FAILURE", ErrorCode: "E101", ErrorMessage: "Insufficient funds", Provider: "Stripe"},
				{Timestamp: time2, RequestID: "req3", MerchantID: "merch1", Status: "RETRY", Provider: "Stripe"}, // Same timestamp, different request
				{Timestamp: time3, RequestID: "req4", MerchantID: "merch3", Status: "SUCCESS", Amount: 200, Currency: "USD", Provider: "PayPal"}, // Latest
			},
			expected: &RetrospectiveReport{
				TotalRequests:        5, // Corrected: 5 log entries
				SuccessfulPayments:   3, // req0, req1, req4
				FailedPayments:       1, // req2
				RetriedAttempts:      1, // req3
				TotalAmountProcessed: 1700, // 500(EUR) + 1000(USD) + 200(USD)
				AmountByCurrency:     map[string]int64{"USD": 1200, "EUR": 500},
				ErrorBreakdown:       map[string]int{"E101": 1},
				ProviderUsage:        map[string]int{"Stripe": 3, "PayPal": 2}, // Stripe: req1, req2, req3. PayPal: req0, req4
				DateFrom:             time4,
				DateTo:               time3,
				ProcessingDuration:   time3.Sub(time4),
			},
		},
		{
			name: "LogsWithNoProviderOrErrorCodes",
			logs: []LogEntry{
				{Timestamp: time1, RequestID: "req1", Status: "SUCCESS", Amount: 100, Currency: "GBP"},
				{Timestamp: time2, RequestID: "req2", Status: "FAILURE", ErrorMessage: "Generic fail"}, // No ErrorCode
				{Timestamp: time3, RequestID: "req3", Status: "RETRY"}, // No provider
			},
			expected: &RetrospectiveReport{
				TotalRequests:        3,
				SuccessfulPayments:   1,
				FailedPayments:       1,
				RetriedAttempts:      1,
				TotalAmountProcessed: 100,
				AmountByCurrency:     map[string]int64{"GBP": 100},
				ErrorBreakdown:       make(map[string]int), // No ErrorCode means nothing in ErrorBreakdown
				ProviderUsage:        make(map[string]int), // No provider means nothing in ProviderUsage
				DateFrom:             time1,
				DateTo:               time3,
				ProcessingDuration:   time3.Sub(time1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := reporter.GenerateRetrospective(tt.logs)

			if (err != nil) != tt.expectError {
				t.Errorf("GenerateRetrospective() error = %v, expectError %v", err, tt.expectError)
				return
			}

			// Ensure all time fields are compared in UTC for consistency.
			// The GenerateRetrospective logic itself should preserve UTC if input is UTC.
			// This step makes the comparison in DeepEqual more robust if there were any
			// subtle timezone shifts (though not expected with current code).
			if report != nil {
				report.DateFrom = report.DateFrom.In(time.UTC)
				report.DateTo = report.DateTo.In(time.UTC)
			}
			if tt.expected != nil {
				tt.expected.DateFrom = tt.expected.DateFrom.In(time.UTC)
				tt.expected.DateTo = tt.expected.DateTo.In(time.UTC)
			}


			if !reflect.DeepEqual(report, tt.expected) {
				t.Errorf("GenerateRetrospective() mismatch (-want +got):\n")
				// Custom diff logic could be more verbose here if needed, e.g. comparing field by field
				// For now, relying on reflect.DeepEqual's output, but it can be dense.
				// A common way to debug DeepEqual failures:
				t.Logf("Got: %+v", report)
				t.Logf("Want: %+v", tt.expected)

				// Detailed map comparisons (optional, if DeepEqual is too broad)
				if report != nil && tt.expected != nil {
					if !reflect.DeepEqual(report.AmountByCurrency, tt.expected.AmountByCurrency) {
						t.Errorf("AmountByCurrency diff:\n  Got: %+v\n Want: %+v", report.AmountByCurrency, tt.expected.AmountByCurrency)
					}
					if !reflect.DeepEqual(report.ErrorBreakdown, tt.expected.ErrorBreakdown) {
						t.Errorf("ErrorBreakdown diff:\n  Got: %+v\n Want: %+v", report.ErrorBreakdown, tt.expected.ErrorBreakdown)
					}
					if !reflect.DeepEqual(report.ProviderUsage, tt.expected.ProviderUsage) {
						t.Errorf("ProviderUsage diff:\n  Got: %+v\n Want: %+v", report.ProviderUsage, tt.expected.ProviderUsage)
					}
					if report.TotalRequests != tt.expected.TotalRequests {
						t.Errorf("TotalRequests diff: Got %d, Want %d", report.TotalRequests, tt.expected.TotalRequests)
					}
					// ... add more field comparisons if needed
				}
			}
		})
	}
}
