package bsql

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	sdkGrant "github.com/conductorone/baton-sdk/pkg/types/grant"
)

// SQLEventFeed implements connectorbuilder.EventFeed for the baton-sql connector.
type SQLEventFeed struct {
	config  Config
	syncers map[string]*SQLSyncer // keyed by resource type ID
}

// NewSQLEventFeed creates a SQLEventFeed from the connector config and resource syncers.
func NewSQLEventFeed(config Config, rawSyncers []connectorbuilder.ResourceSyncer) *SQLEventFeed {
	syncers := make(map[string]*SQLSyncer)
	for _, s := range rawSyncers {
		var syncer *SQLSyncer
		switch v := s.(type) {
		case *SQLSyncer:
			syncer = v
		case *userSyncer:
			syncer = v.SQLSyncer
		}
		if syncer != nil && syncer.resourceType != nil {
			syncers[syncer.resourceType.Id] = syncer
		}
	}
	return &SQLEventFeed{config: config, syncers: syncers}
}

// EventFeedMetadata returns metadata describing this event feed.
func (f *SQLEventFeed) EventFeedMetadata(_ context.Context) *v2.EventFeedMetadata {
	return v2.EventFeedMetadata_builder{
		Id: "sql_event_feed",
		SupportedEventTypes: []v2.EventType{
			v2.EventType_EVENT_TYPE_RESOURCE_CHANGE,
			v2.EventType_EVENT_TYPE_CREATE_GRANT,
			v2.EventType_EVENT_TYPE_CREATE_REVOKE,
		},
	}.Build()
}

// ListEvents returns one page of events from the current source in the cursor.
// earliestEvent is intentionally ignored: each source tracks its own committed cursor via
// sinceForSource, and overriding it with a cross-source lower bound would re-emit already-
// processed events from sources that have advanced past that point.
func (f *SQLEventFeed) ListEvents(
	ctx context.Context,
	earliestEvent *timestamppb.Timestamp,
	pToken *pagination.StreamToken,
) ([]*v2.Event, *pagination.StreamState, annotations.Annotations, error) {
	cursor, err := unmarshalCursor(pToken.Cursor)
	if err != nil {
		return nil, nil, nil, err
	}

	sources := getSources(f.config)
	if len(sources) == 0 {
		return nil, &pagination.StreamState{HasMore: false}, nil, nil
	}

	if cursor.CurrentSourceIdx >= len(sources) {
		cursor.CurrentSourceIdx = 0
	}

	source := sources[cursor.CurrentSourceIdx]

	// Determine `since` for this page. If mid-pagination, use CurrentSince so the WHERE clause
	// stays constant across all pages of the same scan cycle.
	var since time.Time
	if cursor.CurrentSince != "" {
		if t, parseErr := time.Parse(time.RFC3339Nano, cursor.CurrentSince); parseErr == nil {
			since = t
		}
	}
	if since.IsZero() {
		since = f.sinceForSource(cursor, source.Key)
	}

	// Record CurrentSince on the first page of a new cycle (before processing, so a crash
	// mid-page still restarts from the correct since on next invocation).
	if cursor.CurrentSince == "" {
		cursor.CurrentSince = since.UTC().Format(time.RFC3339Nano)
	}

	syncer, hasSyncer := f.syncers[source.ResourceType]
	if !hasSyncer {
		state, stateErr := f.commitAndAdvance(cursor, sources, "", time.Time{})
		if stateErr != nil {
			return nil, nil, nil, stateErr
		}
		return nil, state, nil, nil
	}

	var events []*v2.Event
	var nextPageToken string
	var maxSeen time.Time

	switch source.Kind {
	case incSyncSourceKindResource:
		events, nextPageToken, maxSeen, err = f.processResourceChangePage(ctx, syncer, source, since, pToken.Size, cursor.CurrentPageToken)
	case incSyncSourceKindGrantChanges:
		events, nextPageToken, maxSeen, err = f.processGrantPage(ctx, syncer, source, since, pToken.Size, cursor.CurrentPageToken, false)
	case incSyncSourceKindGrantRevokes:
		events, nextPageToken, maxSeen, err = f.processGrantPage(ctx, syncer, source, since, pToken.Size, cursor.CurrentPageToken, true)
	default:
		err = fmt.Errorf("baton-sql: unknown incremental sync source kind: %s", source.Kind)
	}
	if err != nil {
		return nil, nil, nil, err
	}

	state, err := f.commitAndAdvance(cursor, sources, nextPageToken, maxSeen)
	if err != nil {
		return nil, nil, nil, err
	}
	return events, state, nil, nil
}

// commitAndAdvance updates cursor state after processing a page and builds the StreamState.
// maxSeen is accumulated into cursor.CurrentMaxSeen; on source exhaustion the committed cursor advances.
func (f *SQLEventFeed) commitAndAdvance(
	cursor *eventFeedCursor,
	sources []incSyncSource,
	nextPageToken string,
	maxSeen time.Time,
) (*pagination.StreamState, error) {
	source := sources[cursor.CurrentSourceIdx]

	// Accumulate max seen across pages of the same cycle.
	if !maxSeen.IsZero() {
		existing := time.Time{}
		if cursor.CurrentMaxSeen != "" {
			if t, err := time.Parse(time.RFC3339Nano, cursor.CurrentMaxSeen); err == nil {
				existing = t
			}
		}
		if maxSeen.After(existing) {
			cursor.CurrentMaxSeen = maxSeen.UTC().Format(time.RFC3339Nano)
		}
	}

	hasMore := true
	if nextPageToken != "" {
		// More pages remain for this source.
		cursor.CurrentPageToken = nextPageToken
	} else {
		// Source exhausted: commit accumulated max seen, then advance to next source.
		if cursor.CurrentMaxSeen != "" {
			cursor.SourceCursors[source.Key] = cursor.CurrentMaxSeen
		}
		cursor.CurrentSince = ""
		cursor.CurrentMaxSeen = ""
		cursor.CurrentPageToken = ""
		cursor.CurrentSourceIdx++
		if cursor.CurrentSourceIdx >= len(sources) {
			cursor.CurrentSourceIdx = 0
			hasMore = false
		}
	}

	cursorStr, err := marshalCursor(cursor)
	if err != nil {
		return nil, err
	}
	return &pagination.StreamState{Cursor: cursorStr, HasMore: hasMore}, nil
}

// sinceForSource returns the starting timestamp for the given source key.
// Each source independently tracks its own committed cursor; callers must not override this
// with cross-source values (e.g. earliestEvent) as that would skip unprocessed events.
func (f *SQLEventFeed) sinceForSource(cursor *eventFeedCursor, key string) time.Time {
	if ts, ok := cursor.SourceCursors[key]; ok {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return t
		}
	}
	return time.Now().UTC().Add(-defaultLookback(f.config.IncrementalSync))
}

// processResourceChangePage runs one page of the resource incremental query and returns ResourceChangeEvents.
func (f *SQLEventFeed) processResourceChangePage(
	ctx context.Context,
	s *SQLSyncer,
	source incSyncSource,
	since time.Time,
	pageSize int,
	pageToken string,
) ([]*v2.Event, string, time.Time, error) {
	rc := source.ResConfig

	vars, err := s.PrepareQueryVars(ctx, nil, rc.Vars)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("baton-sql: failed to prepare vars for resource incremental sync: %w", err)
	}
	vars[sinceKey] = since

	pToken := &pagination.Token{Token: pageToken}
	if pageSize > 0 {
		pToken.Size = pageSize
	}

	var events []*v2.Event
	var maxSeen time.Time

	resourceIDExpr := s.config.List.Map.Id
	if rc.ResourceId != "" {
		resourceIDExpr = rc.ResourceId
	}

	npt, err := s.runQuery(ctx, pToken, rc.Query, rc.Pagination, vars, func(ctx context.Context, rowMap map[string]any) (bool, error) {
		inputs := s.env.SyncInputs(rowMap)
		resourceID, evalErr := s.env.EvaluateString(ctx, resourceIDExpr, inputs)
		if evalErr != nil {
			return false, fmt.Errorf("baton-sql: failed to evaluate resource ID in resource incremental sync: %w", evalErr)
		}

		rowTimestamp := since
		if ts, ok := rowMap[rc.CursorColumn]; ok {
			t, parseErr := toTime(ts)
			if parseErr != nil {
				return false, fmt.Errorf("baton-sql: failed to parse cursor column %q value %v in resource incremental sync: %w", rc.CursorColumn, ts, parseErr)
			}
			rowTimestamp = t
			if t.After(maxSeen) {
				maxSeen = t
			}
		}

		rowKey := grantRowKey(rowMap, rc.Pagination)
		event := v2.Event_builder{
			Id:         fmt.Sprintf("resource:%s:%s:%s:%s", source.ResourceType, resourceID, rowTimestamp.UTC().Format(time.RFC3339Nano), rowKey),
			OccurredAt: timestamppb.New(rowTimestamp),
			ResourceChangeEvent: v2.ResourceChangeEvent_builder{
				ResourceId: &v2.ResourceId{
					ResourceType: source.ResourceType,
					Resource:     resourceID,
				},
			}.Build(),
		}.Build()
		events = append(events, event)
		return true, nil
	})
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("baton-sql: failed to list resource changes: %w", err)
	}

	return events, npt, maxSeen, nil
}

// processGrantPage runs one page of a grant changes or revokes query and returns grant/revoke events.
func (f *SQLEventFeed) processGrantPage(
	ctx context.Context,
	s *SQLSyncer,
	source incSyncSource,
	since time.Time,
	pageSize int,
	pageToken string,
	isRevoke bool,
) ([]*v2.Event, string, time.Time, error) {
	gc := source.GrantConfig

	query := gc.ChangesQuery
	cursorCol := gc.ChangesCursorColumn
	if isRevoke {
		query = gc.RevokesQuery
		cursorCol = gc.RevokesCursorColumn
	}

	vars, err := s.PrepareQueryVars(ctx, nil, gc.Vars)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("baton-sql: failed to prepare vars for grant incremental sync: %w", err)
	}
	vars[sinceKey] = since

	pToken := &pagination.Token{Token: pageToken}
	if pageSize > 0 {
		pToken.Size = pageSize
	}

	var events []*v2.Event
	var maxSeen time.Time

	npt, err := s.runQuery(ctx, pToken, query, gc.Pagination, vars, func(ctx context.Context, rowMap map[string]any) (bool, error) {
		rowTimestamp := since
		if ts, ok := rowMap[cursorCol]; ok {
			t, parseErr := toTime(ts)
			if parseErr != nil {
				return false, fmt.Errorf("baton-sql: failed to parse cursor column %q value %v in grant incremental sync: %w", cursorCol, ts, parseErr)
			}
			rowTimestamp = t
			if t.After(maxSeen) {
				maxSeen = t
			}
		}

		inputs := s.env.SyncInputs(rowMap)
		resourceID, evalErr := s.env.EvaluateString(ctx, gc.ResourceId, inputs)
		if evalErr != nil {
			return false, fmt.Errorf("baton-sql: failed to evaluate resource_id in grant incremental sync: %w", evalErr)
		}

		minimalResource := &v2.Resource{
			Id: &v2.ResourceId{ResourceType: source.ResourceType, Resource: resourceID},
		}

		for _, mapping := range source.GrantMap {
			grant, ok, mapErr := f.mapGrantFromRow(ctx, s, minimalResource, mapping, rowMap)
			if mapErr != nil {
				return false, mapErr
			}
			if !ok {
				continue
			}

			principalID := grant.GetPrincipal().GetId().GetResource()
			tsStr := rowTimestamp.UTC().Format(time.RFC3339Nano)
			rowKey := grantRowKey(rowMap, gc.Pagination)

			var event *v2.Event
			if isRevoke {
				event = v2.Event_builder{
					Id:         fmt.Sprintf("revoke:%s:%s:%s:%s:%s", source.ResourceType, resourceID, principalID, tsStr, rowKey),
					OccurredAt: timestamppb.New(rowTimestamp),
					CreateRevokeEvent: v2.CreateRevokeEvent_builder{
						Entitlement: grant.GetEntitlement(),
						Principal:   grant.GetPrincipal(),
					}.Build(),
				}.Build()
			} else {
				event = v2.Event_builder{
					Id:         fmt.Sprintf("grant:%s:%s:%s:%s:%s", source.ResourceType, resourceID, principalID, tsStr, rowKey),
					OccurredAt: timestamppb.New(rowTimestamp),
					CreateGrantEvent: v2.CreateGrantEvent_builder{
						Entitlement: grant.GetEntitlement(),
						Principal:   grant.GetPrincipal(),
					}.Build(),
				}.Build()
			}
			events = append(events, event)
		}
		return true, nil
	})
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("baton-sql: failed to list grant changes: %w", err)
	}

	return events, npt, maxSeen, nil
}

// mapGrantFromRow maps a row to a grant for the incremental path.
// Uses a minimal resource with only ID set — trait fields are unavailable in incremental sync.
// CEL expressions in the grant mapping must not reference resource trait fields (display_name, etc.).
func (f *SQLEventFeed) mapGrantFromRow(
	ctx context.Context,
	s *SQLSyncer,
	minimalResource *v2.Resource,
	mapping *GrantMapping,
	rowMap map[string]any,
) (*v2.Grant, bool, error) {
	inputs := s.env.SyncInputsWithResource(rowMap, minimalResource)

	if mapping.SkipIf != "" {
		skip, err := s.env.EvaluateBool(ctx, mapping.SkipIf, inputs)
		if err != nil {
			return nil, false, err
		}
		if skip {
			return nil, false, nil
		}
	}

	principalID, err := s.env.EvaluateString(ctx, mapping.PrincipalId, inputs)
	if err != nil {
		return nil, false, err
	}
	if principalID == "" {
		return nil, false, nil
	}

	entitlementID, err := s.env.EvaluateString(ctx, mapping.Entitlement, inputs)
	if err != nil {
		return nil, false, err
	}
	if entitlementID == "" {
		return nil, false, nil
	}

	principal := &v2.ResourceId{
		ResourceType: mapping.PrincipalType,
		Resource:     principalID,
	}

	return sdkGrant.NewGrant(minimalResource, entitlementID, principal), true, nil
}

// grantRowKey returns the string representation of the pagination primary key value for a row.
// It is used to make grant/revoke event IDs unique when multiple rows share the same
// (resource, principal, timestamp) triple — e.g. bulk imports with second-precision timestamps.
func grantRowKey(rowMap map[string]any, p *Pagination) string {
	if p == nil || p.PrimaryKey == "" {
		return ""
	}
	v, ok := rowMap[p.PrimaryKey]
	if !ok {
		return ""
	}

	switch n := v.(type) {
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toTime converts a database column value to time.Time.
func toTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case []byte:
		pt, err := parseTime(string(t))
		if err != nil {
			return time.Time{}, err
		}
		return *pt, nil
	case string:
		pt, err := parseTime(t)
		if err != nil {
			return time.Time{}, err
		}
		return *pt, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time type %T", v)
	}
}
