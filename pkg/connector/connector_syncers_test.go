package connector

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The postgres driver opens lazily, so these configs build a connector without a live database.
const syncersTestConnect = `
connect:
  dsn: postgres://127.0.0.1:1/db
`

const syncersTestUser = `
  user:
    name: User
    list:
      query: SELECT id FROM users
      map:
        id: ".id"
        display_name: ".id"
        traits:
          user: {}
`

func TestNewFromYAML_SyncerBuildError(t *testing.T) {
	yaml := []byte(syncersTestConnect + `
resource_types:
  user:
    name: User
`)
	_, err := NewFromYAML(t.Context(), yaml, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "baton-sql: failed to create resource syncers")
	require.Contains(t, err.Error(), "has no listing defined")
}

func TestNewFromYAML_NoEventFeed(t *testing.T) {
	c, err := NewFromYAML(t.Context(), []byte(syncersTestConnect+"resource_types:"+syncersTestUser), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	require.Len(t, c.ResourceSyncers(t.Context()), 1)
	require.Nil(t, c.EventFeeds(t.Context()))
	require.Nil(t, c.eventFeedSyncers)
}

func TestNewFromYAML_EventFeedUsesSeparateSyncers(t *testing.T) {
	yaml := []byte(syncersTestConnect + "resource_types:" + syncersTestUser + `    get:
      query: SELECT id FROM users WHERE id = ?<resource_id>
    event_feed:
      query: SELECT id, updated_at FROM users WHERE updated_at > ?<since> ORDER BY updated_at, id LIMIT ?<Limit> OFFSET ?<Offset>
      cursor_column: updated_at
      pagination:
        strategy: offset
        primary_key: id
`)
	c, err := NewFromYAML(t.Context(), yaml, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	require.Len(t, c.EventFeeds(t.Context()), 1)

	syncers := c.ResourceSyncers(t.Context())
	require.Len(t, syncers, 1)
	require.Len(t, c.eventFeedSyncers, 1)
	require.NotSame(t, syncers[0], c.eventFeedSyncers[0])
}
