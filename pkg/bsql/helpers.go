package bsql

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/conductorone/baton-sql/pkg/database"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/crypto"
)

// Common time formats that may be used in databases.
var timeFormats = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",           // MySQL, PostgreSQL
	"2006-01-02 15:04:05.000",       // MySQL, SQL Server
	"2006-01-02 15:04:05.000000",    // PostgreSQL
	"2006-01-02 15:04:05.000000000", // Precision formats
	"2006-01-02T15:04:05Z",          // ISO8601
	"2006-01-02T15:04:05.000Z",      // ISO8601 with ms
	"2006-01-02T15:04:05.000-07:00", // ISO8601 with timezone
	"2006-01-02",                    // Date only
	"01/02/2006 15:04:05",           // US format
	"02/01/2006 15:04:05",           // European format
	"01/02/2006",                    // US short date
	"02/01/2006",                    // European short date
	"Jan 02, 2006 15:04:05",         // Oracle format
	"02-JAN-2006 15:04:05",          // Oracle format (uppercase month)
	"02-Jan-2006 15:04:05",          // Oracle format (mixed case month)
	"02-JAN-06 15:04:05",            // Oracle short year
	"02-Jan-06 15:04:05",            // Oracle short year (mixed case)
	"02-01-2006",                    // Oracle date only (day-month-year)
	"02-01-06",                      // Oracle date only (day-month-short year)
	"January 02, 2006 15:04:05",     // Long month name
	"2006-01-02-15.04.05.000000",    // DB2 format
	time.UnixDate,
	time.ANSIC,
	time.RubyDate,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RFC1123,
	time.RFC1123Z,
	time.Stamp,
	time.StampMilli,
	time.StampMicro,
	time.StampNano,
}

// parseTime attempts to parse a time string using various database formats.
func parseTime(value string) (*time.Time, error) {
	// Handle empty string
	if value == "" {
		return &time.Time{}, errors.New("empty time string")
	}

	value = strings.TrimSpace(value)

	// Try all the predefined formats
	for _, format := range timeFormats {
		if t, err := time.Parse(format, value); err == nil {
			return &t, nil
		}
	}

	// Try Unix timestamp (seconds since epoch)
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		// Check if it's likely a Unix timestamp (Jan 1, 1970 to Jan 1, 2100)
		if i > 0 && i < 4102444800 {
			t := time.Unix(i, 0)
			return &t, nil
		}
	}

	// Try millisecond precision Unix timestamp
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		// Check if it's likely a millisecond timestamp
		if i > 1000000000000 && i < 4102444800000 {
			t := time.Unix(i/1000, (i%1000)*1000000)
			return &t, nil
		}
	}

	return nil, errors.New("unable to parse time string with any known format")
}

// parseTimeWithEngine attempts to parse a time string using database-specific formats based on engine type.
func parseTimeWithEngine(value string, dbEngine database.DbEngine) (*time.Time, error) {
	// Handle empty string
	if value == "" {
		return &time.Time{}, errors.New("empty time string")
	}

	value = strings.TrimSpace(value)

	// Try formats prioritized based on database engine
	var prioritizedFormats []string

	switch dbEngine {
	case database.MySQL:
		// MySQL common formats
		prioritizedFormats = []string{
			"2006-01-02 15:04:05",
			"2006-01-02 15:04:05.000",
			time.RFC3339,
		}
	case database.PostgreSQL:
		// PostgreSQL common formats
		prioritizedFormats = []string{
			"2006-01-02 15:04:05.000000",
			"2006-01-02 15:04:05",
			time.RFC3339,
		}
	case database.Oracle:
		// Oracle common formats
		prioritizedFormats = []string{
			"02-JAN-2006 15:04:05",
			"02-Jan-2006 15:04:05",
			"Jan 02, 2006 15:04:05",
		}
	default:
		// Try the generic time parser for unknown engines
		return parseTime(value)
	}

	// Try prioritized formats first
	for _, format := range prioritizedFormats {
		if t, err := time.Parse(format, value); err == nil {
			return &t, nil
		}
	}

	// Fall back to the generic parser if prioritized formats don't match
	return parseTime(value)
}

// generateCredentials generates a random password based on the credential options and configuration.
func generateCredentials(credentialOptions *v2.CredentialOptions) (string, error) {
	if credentialOptions == nil || credentialOptions.GetRandomPassword() == nil {
		return "", errors.New("unsupported credential option: only random password is supported")
	}

	randomPasswordOpts := credentialOptions.GetRandomPassword()

	password, err := crypto.GenerateRandomPassword(randomPasswordOpts)
	if err != nil {
		return "", err
	}
	return password, nil
}
