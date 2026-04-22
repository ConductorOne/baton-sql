package bsql

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const defaultLookbackDuration = 3 * time.Hour

// eventFeedCursor tracks pagination state across multiple ListEvents() calls.
//
// SourceCursors holds a committed "since" timestamp per source key, advanced only when the source
// is fully exhausted (all pages processed). CurrentSince and CurrentMaxSeen are transient state
// for the in-progress scan cycle so that `since` stays constant across pages of the same cycle.
type eventFeedCursor struct {
	SourceCursors    map[string]string `json:"source_cursors"`
	CurrentSourceIdx int               `json:"current_source_idx"`
	CurrentPageToken string            `json:"current_page_token"`
	// CurrentSince is the since value in use for the current scan cycle (set on first page).
	CurrentSince string `json:"current_since,omitempty"`
	// CurrentMaxSeen accumulates the max cursor-column timestamp seen so far in the current cycle.
	CurrentMaxSeen string `json:"current_max_seen,omitempty"`
}

func marshalCursor(c *eventFeedCursor) (string, error) {
	if c == nil {
		return "", nil
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("baton-sql: failed to marshal event feed cursor: %w", err)
	}
	return string(data), nil
}

func unmarshalCursor(s string) (*eventFeedCursor, error) {
	if s == "" {
		return &eventFeedCursor{SourceCursors: make(map[string]string)}, nil
	}
	c := &eventFeedCursor{}
	if err := json.Unmarshal([]byte(s), c); err != nil {
		return nil, fmt.Errorf("baton-sql: failed to unmarshal event feed cursor: %w", err)
	}
	if c.SourceCursors == nil {
		c.SourceCursors = make(map[string]string)
	}
	return c, nil
}

type incSyncSourceKind string

const (
	incSyncSourceKindResource     incSyncSourceKind = "resource"
	incSyncSourceKindGrantChanges incSyncSourceKind = "grant_changes"
	incSyncSourceKindGrantRevokes incSyncSourceKind = "grant_revokes"
)

type incSyncSource struct {
	Key          string
	Kind         incSyncSourceKind
	ResourceType string // resource type ID
	GrantIdx     int    // index into rt.Grants (grant event sources only)
	ResConfig    *ResourceIncrementalSync
	GrantConfig  *GrantsIncrementalSync
	GrantMap     []*GrantMapping
}

// getSources enumerates all configured incremental sources in deterministic order.
// Order: resource type IDs sorted lexicographically, then per-type: resource, grant changes, grant revokes.
func getSources(config Config) []incSyncSource {
	rtIDs := make([]string, 0, len(config.ResourceTypes))
	for id := range config.ResourceTypes {
		rtIDs = append(rtIDs, id)
	}
	sort.Strings(rtIDs)

	var sources []incSyncSource
	for _, rtID := range rtIDs {
		rt := config.ResourceTypes[rtID]
		if rt.IncrementalSync != nil {
			sources = append(sources, incSyncSource{
				Key:          fmt.Sprintf("%s:resource", rtID),
				Kind:         incSyncSourceKindResource,
				ResourceType: rtID,
				ResConfig:    rt.IncrementalSync,
			})
		}
		for i, gq := range rt.Grants {
			if gq.IncrementalSync == nil {
				continue
			}
			gs := gq.IncrementalSync
			sources = append(sources, incSyncSource{
				Key:          fmt.Sprintf("%s:grants:%d:changes", rtID, i),
				Kind:         incSyncSourceKindGrantChanges,
				ResourceType: rtID,
				GrantIdx:     i,
				GrantConfig:  gs,
				GrantMap:     gq.Map,
			})
			if gs.RevokesQuery != "" {
				sources = append(sources, incSyncSource{
					Key:          fmt.Sprintf("%s:grants:%d:revokes", rtID, i),
					Kind:         incSyncSourceKindGrantRevokes,
					ResourceType: rtID,
					GrantIdx:     i,
					GrantConfig:  gs,
					GrantMap:     gq.Map,
				})
			}
		}
	}
	return sources
}

// defaultLookback returns the configured lookback duration, defaulting to 3 hours.
func defaultLookback(config *IncrementalSyncConfig) time.Duration {
	if config != nil && config.DefaultLookback != "" {
		d, err := time.ParseDuration(config.DefaultLookback)
		if err == nil {
			return d
		}
	}
	return defaultLookbackDuration
}
