package bsql

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-sdk/pkg/pagination"
)

// iterateDBs runs work against one database at a time and stitches the per-database
// pagination tokens into a single outer token.
//
// Single-DB connectors (and any query marked scope: cluster) hit the fast path: work
// runs once against the primary handle and its token is returned unchanged, so tokens
// minted before the multi-database feature existed stay wire-compatible.
//
// Multi-DB connectors use a pagination.Bag holding one PageState per database; each
// call drains the current database's pages before advancing to the next. s.db and
// s.currentDBName are swapped in place before invoking work, which is safe because
// the SDK calls List/Entitlements/Grants serially per syncer.
func (s *SQLSyncer) iterateDBs(
	ctx context.Context,
	scope string,
	pToken *pagination.Token,
	work func(ctx context.Context, dbName string, innerToken *pagination.Token) (string, error),
) (string, error) {
	if pToken == nil {
		pToken = &pagination.Token{}
	}

	if scope == scopeCluster || len(s.dbNames) <= 1 {
		if err := s.setCurrentDB(s.primaryDBName); err != nil {
			return "", err
		}
		return work(ctx, s.primaryDBName, pToken)
	}

	b := &pagination.Bag{}
	if err := b.Unmarshal(pToken.Token); err != nil {
		return "", err
	}

	if b.Current() == nil {
		// pagination.Bag is LIFO — push reversed so iteration follows sorted dbNames.
		for i := len(s.dbNames) - 1; i >= 0; i-- {
			b.Push(pagination.PageState{
				ResourceTypeID: dbIterPageStateType,
				ResourceID:     s.dbNames[i],
			})
		}
	}

	current := b.Current()
	if current == nil {
		return "", nil
	}
	if current.ResourceTypeID != dbIterPageStateType {
		return "", fmt.Errorf("iterateDBs: unexpected page state type %q", current.ResourceTypeID)
	}

	dbName := current.ResourceID
	if err := s.setCurrentDB(dbName); err != nil {
		return "", err
	}
	innerToken := &pagination.Token{
		Size:  pToken.Size,
		Token: current.Token,
	}

	npt, err := work(ctx, dbName, innerToken)
	if err != nil {
		return "", err
	}

	if err := b.Next(npt); err != nil {
		return "", err
	}
	return b.Marshal()
}
