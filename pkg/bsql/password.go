package bsql

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
)

// Mirrors the character classes used by baton-sdk's crypto.GenerateRandomPassword.
// Kept in sync with vendor/github.com/conductorone/baton-sdk/pkg/crypto/password.go.
const (
	pwUpperCaseLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	pwLowerCaseLetters = "abcdefghijklmnopqrstuvwxyz"
	pwDigits           = "0123456789"
	pwSymbols          = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
)

var (
	errEmptyPasswordPool         = errors.New("disallowed_characters removed every valid password character")
	errEmptyConstraintCharSet    = errors.New("disallowed_characters removed every character from a password constraint")
	errInvalidFilteredPwdLength  = errors.New("invalid password length")
	errPasswordLengthBelowMinima = errors.New("password length is less than the sum of constraint minimums")
)

// generateRandomPasswordFiltered mirrors crypto.GenerateRandomPassword's algorithm
// but filters every char set by the runes in disallowed before drawing from it.
// When disallowed is empty, callers should use the SDK directly instead.
func generateRandomPasswordFiltered(opts *v2.LocalCredentialOptions_RandomPassword, disallowed string) (string, error) {
	if opts == nil {
		return "", errors.New("random password options are required")
	}

	length := opts.GetLength()
	if length < 8 {
		return "", errInvalidFilteredPwdLength
	}

	disallowedSet := make(map[rune]struct{}, len(disallowed))
	for _, r := range disallowed {
		disallowedSet[r] = struct{}{}
	}

	upper := filterRunes(pwUpperCaseLetters, disallowedSet)
	lower := filterRunes(pwLowerCaseLetters, disallowedSet)
	digits := filterRunes(pwDigits, disallowedSet)
	symbols := filterRunes(pwSymbols, disallowedSet)
	pool := upper + lower + digits + symbols
	if pool == "" {
		return "", errEmptyPasswordPool
	}

	var password strings.Builder
	if constraints := opts.GetConstraints(); len(constraints) > 0 {
		for _, c := range constraints {
			set := filterRunes(c.GetCharSet(), disallowedSet)
			if set == "" {
				return "", errEmptyConstraintCharSet
			}
			for i := uint32(0); i < c.GetMinCount(); i++ {
				if err := writeRandomChar(&password, set); err != nil {
					return "", err
				}
			}
		}
	} else {
		for _, set := range []string{upper, lower, digits, symbols} {
			if set == "" {
				continue
			}
			if err := writeRandomChar(&password, set); err != nil {
				return "", err
			}
		}
	}

	remaining := length - int64(password.Len())
	if remaining < 0 {
		return "", fmt.Errorf("%w: length=%d, minimums=%d", errPasswordLengthBelowMinima, length, password.Len())
	}
	for i := int64(0); i < remaining; i++ {
		if err := writeRandomChar(&password, pool); err != nil {
			return "", err
		}
	}

	return shuffleString(password.String())
}

func filterRunes(s string, disallowed map[rune]struct{}) string {
	if len(disallowed) == 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if _, banned := disallowed[r]; banned {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func writeRandomChar(b *strings.Builder, set string) error {
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
	if err != nil {
		return fmt.Errorf("failed to generate password: %w", err)
	}
	if err := b.WriteByte(set[idx.Int64()]); err != nil {
		return fmt.Errorf("failed to generate password: %w", err)
	}
	return nil
}

func shuffleString(s string) (string, error) {
	runes := []rune(s)
	for i := len(runes) - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", fmt.Errorf("failed to shuffle password: %w", err)
		}
		j := int(jBig.Int64())
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes), nil
}
