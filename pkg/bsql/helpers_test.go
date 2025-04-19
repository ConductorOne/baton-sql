package bsql

import (
	"testing"
	"time"

	"github.com/conductorone/baton-sql/pkg/database"
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
			input:         "2025-04-17 14:30:45",
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Oracle format (uppercase)",
			input:         "17-APR-2025 14:30:45",
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Oracle format (mixed case)",
			input:         "17-Apr-2025 14:30:45",
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Oracle format with short year",
			input:         "17-APR-25 14:30:45",
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "ISO format",
			input:         "2025-04-17T14:30:45Z",
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Unix timestamp",
			input:         "1744900245",
			expected:      time.Unix(1744900245, 0),
			expectSuccess: true,
		},
		{
			name:          "Millisecond timestamp",
			input:         "1744900245000",
			expected:      time.Unix(1744900245, 0),
			expectSuccess: true,
		},
		{
			name:          "Date only",
			input:         "2025-04-17",
			expected:      time.Date(2025, 4, 17, 0, 0, 0, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "US format",
			input:         "04/17/2025 14:30:45",
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
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

func TestParseTimeWithEngine(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		dbEngine      database.DbEngine
		expected      time.Time
		expectSuccess bool
	}{
		{
			name:          "MySQL format with MySQL engine",
			input:         "2025-04-17 14:30:45",
			dbEngine:      database.MySQL,
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Oracle format with Oracle engine",
			input:         "17-APR-2025 14:30:45",
			dbEngine:      database.Oracle,
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "PostgreSQL format with PostgreSQL engine",
			input:         "2025-04-17 14:30:45.123456",
			dbEngine:      database.PostgreSQL,
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 123456000, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "ISO format with any engine",
			input:         "2025-04-17T14:30:45Z",
			dbEngine:      database.MySQL,
			expected:      time.Date(2025, 4, 17, 14, 30, 45, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "Invalid format with any engine",
			input:         "not a date",
			dbEngine:      database.MySQL,
			expectSuccess: false,
		},
		{
			name:          "Empty string with any engine",
			input:         "",
			dbEngine:      database.MySQL,
			expectSuccess: false,
		},
		{
			name:          "NULL_VALUE string with any engine",
			input:         "NULL_VALUE",
			dbEngine:      database.MySQL,
			expectSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTimeWithEngine(tt.input, tt.dbEngine)
			if tt.expectSuccess {
				require.NoError(t, err)
				require.True(t, result.Equal(tt.expected), "Expected %v, got %v", tt.expected, result)
			} else {
				require.Error(t, err)
			}
		})
	}
}
