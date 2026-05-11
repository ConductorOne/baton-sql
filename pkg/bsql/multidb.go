package bsql

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-sdk/pkg/pagination"
)

// iterateDBs is a no-op (single-DB or scope: cluster) or a pagination.Bag-driven
// fan-out (multi-DB). The single-DB path returns the inner token byte-for-byte so
// pre-multi-database token formats stay wire-compatible.
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
