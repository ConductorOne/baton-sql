package bsql

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expected      time.Time
		expectSuccess bool
	}{
		{
			name:          "MySQL format",
			input:         "2023-04-17 14:30:45",
			expected:      time.Date(2023, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Oracle format (uppercase)",
			input:         "17-APR-2023 14:30:45",
			expected:      time.Date(2023, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Oracle format (mixed case)",
			input:         "17-Apr-2023 14:30:45",
			expected:      time.Date(2023, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Oracle format with short year",
			input:         "17-APR-23 14:30:45",
			expected:      time.Date(2023, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "ISO format",
			input:         "2023-04-17T14:30:45Z",
			expected:      time.Date(2023, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Unix timestamp",
			input:         "1681742245",
			expected:      time.Unix(1681742245, 0),
			expectSuccess: true,
		},
		{
			name:          "Millisecond timestamp",
			input:         "1681742245000",
			expected:      time.Unix(1681742245, 0),
			expectSuccess: true,
		},
		{
			name:          "Date only",
			input:         "2023-04-17",
			expected:      time.Date(2023, 4, 17, 0, 0, 0, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "US format",
			input:         "04/17/2023 14:30:45",
			expected:      time.Date(2023, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Invalid format",
			input:         "not a date",
			expectSuccess: false,
		},
		{
			name:          "Empty string",
			input:         "",
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTime(tt.input)
			if tt.expectSuccess {
				require.NoError(t, err)
				require.True(t, result.Equal(tt.expected), "Expected %v, got %v", tt.expected, result)
			} else {
				require.Error(t, err)
			}
		})
	}
}
