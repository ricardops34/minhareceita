package graph

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"codeberg.org/cuducos/minha-receita/db"
	"github.com/dgraph-io/badger/v4"
	"github.com/schollz/progressbar/v3"
)

type database interface {
	StreamRelationships(context.Context, func(db.Relationship) error) error
	RelationshipCount(context.Context) (int64, error)
}

func Create(ctx context.Context, d database, t int64, pth string, bar *progressbar.ProgressBar) error {
	if err := os.RemoveAll(pth); err != nil {
		return fmt.Errorf("could not remove existing graph directory: %w", err)
	}

	opts := badger.DefaultOptions(pth).WithLoggingLevel(badger.WARNING)
	kv, err := badger.Open(opts)
	if err != nil {
		return fmt.Errorf("could not open badger: %w", err)
	}
	defer func() {
		if err := kv.Close(); err != nil {
			slog.Error("could not close badger", "error", err)
		}
	}()

	w := kv.NewWriteBatch()
	defer w.Cancel()

	err = d.StreamRelationships(ctx, func(r db.Relationship) error {
		c, err := r.EncodeCompany()
		if err != nil {
			return fmt.Errorf("failed to encode company metadata: %w", err)
		}
		if err := w.Set(append([]byte("meta:"), r.CompanyID...), c); err != nil {
			return fmt.Errorf("failed to write company metadata: %w", err)
		}

		p, err := r.EncodePartner()
		if err != nil {
			return fmt.Errorf("failed to encode partner metadata: %w", err)
		}
		if err := w.Set(append([]byte("meta:"), r.PartnerID...), p); err != nil {
			return fmt.Errorf("failed to write partner metadata: %w", err)
		}

		k1 := make([]byte, 6+len(r.CompanyID)+len(r.PartnerID))
		copy(k1[0:4], "rel:")
		copy(k1[4:4+len(r.CompanyID)], r.CompanyID)
		copy(k1[4+len(r.CompanyID):6+len(r.CompanyID)], "->")
		copy(k1[6+len(r.CompanyID):], r.PartnerID)
		if err := w.Set(k1, []byte{}); err != nil {
			return fmt.Errorf("failed to write company relationship: %w", err)
		}

		k2 := make([]byte, 6+len(r.CompanyID)+len(r.PartnerID))
		copy(k2[0:4], "rel:")
		copy(k2[4:4+len(r.PartnerID)], r.PartnerID)
		copy(k2[4+len(r.PartnerID):6+len(r.PartnerID)], "<-")
		copy(k2[6+len(r.PartnerID):], r.CompanyID)
		if err := w.Set(k2, []byte{}); err != nil {
			return fmt.Errorf("failed to write partner relationship: %w", err)
		}

		if bar != nil {
			if err := bar.Add(1); err != nil {
				slog.Warn("could not update the progress bar", "error", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err = w.Flush(); err != nil {
		return fmt.Errorf("error flushing badger write batch: %w", err)
	}

	return nil
}
