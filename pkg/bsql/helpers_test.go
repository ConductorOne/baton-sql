package bsql

import (
	"testing"
	"time"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
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

func TestGenerateCredentials(t *testing.T) {
	tests := []struct {
		name              string
		credentialOptions *v2.CredentialOptions
		expectError       bool
		expectNonEmpty    bool
	}{
		{
			name:        "nil credential options",
			expectError: true,
		},
		{
			name: "no random password",
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_NoPassword_{
					NoPassword: &v2.CredentialOptions_NoPassword{},
				},
			},
			expectError: true,
		},
		{
			name: "valid random password with constraints",
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 16,
						Constraints: []*v2.PasswordConstraint{
							{
								CharSet:  "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
								MinCount: 2,
							},
							{
								CharSet:  "abcdefghijklmnopqrstuvwxyz",
								MinCount: 2,
							},
							{
								CharSet:  "0123456789",
								MinCount: 1,
							},
							{
								CharSet:  "!@#$%^&*()_+-=[]{}|;:,.<>?",
								MinCount: 1,
							},
						},
					},
				},
			},
			expectError:    false,
			expectNonEmpty: true,
		},
		{
			name: "simple random password without constraints",
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 20,
					},
				},
			},
			expectError:    false,
			expectNonEmpty: true,
		},
		{
			name: "very long password",
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 32,
						Constraints: []*v2.PasswordConstraint{
							{
								CharSet:  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789",
								MinCount: 5,
							},
						},
					},
				},
			},
			expectError:    false,
			expectNonEmpty: true,
		},
		{
			name: "minimum practical length",
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 8,
					},
				},
			},
			expectError:    false,
			expectNonEmpty: true,
		},
		{
			name: "database-style password (medium length)",
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 20,
						Constraints: []*v2.PasswordConstraint{
							{
								CharSet:  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz",
								MinCount: 8,
							},
							{
								CharSet:  "0123456789",
								MinCount: 2,
							},
							{
								CharSet:  "!@#$%^&*()_+-=[]{}|;:,.<>?",
								MinCount: 4,
							},
						},
					},
				},
			},
			expectError:    false,
			expectNonEmpty: true,
		},
		{
			name: "enterprise-grade password",
			credentialOptions: &v2.CredentialOptions{
				Options: &v2.CredentialOptions_RandomPassword_{
					RandomPassword: &v2.CredentialOptions_RandomPassword{
						Length: 24,
						Constraints: []*v2.PasswordConstraint{
							{
								CharSet:  "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
								MinCount: 3,
							},
							{
								CharSet:  "abcdefghijklmnopqrstuvwxyz",
								MinCount: 3,
							},
							{
								CharSet:  "0123456789",
								MinCount: 2,
							},
							{
								CharSet:  "!@#$%^&*",
								MinCount: 2,
							},
						},
					},
				},
			},
			expectError:    false,
			expectNonEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			password, err := generateCredentials(tt.credentialOptions)

			if tt.expectError {
				require.Error(t, err)
				require.Empty(t, password)
				return
			}

			require.NoError(t, err)
			if tt.expectNonEmpty {
				require.NotEmpty(t, password)
				// Verify the password length matches the requested length
				if tt.credentialOptions.GetRandomPassword() != nil {
					expectedLength := tt.credentialOptions.GetRandomPassword().GetLength()
					if expectedLength > 0 {
						require.Equal(t, int(expectedLength), len(password), "Password length should match requested length")
					}
				}
			}
		})
	}
}
