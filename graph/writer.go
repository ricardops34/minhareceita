package graph

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"codeberg.org/cuducos/minha-receita/company"
	"github.com/dgraph-io/badger/v4"
	"tangled.org/cuducos.me/go-cnpj"
)

var DefaultGraphPath = filepath.Join("data", "graph.db")

type Writer struct {
	path  string
	kv    *badger.DB
	batch *badger.WriteBatch
}

func (w *Writer) Path() string { return w.path }

func (w *Writer) Close() error {
	if err := w.batch.Flush(); err != nil {
		return errors.Join(err, w.kv.Close())
	}
	return w.kv.Close()
}

func (w *Writer) Save(log *slog.Logger, r *company.Relationship) error {
	if r.PartnerType == 0 && !cnpj.IsValid(r.PartnerID) && log != nil {
		log.Warn("Invalid partner CNPJ", "company", r.CompanyID, "partner", r.PartnerID)
	}
	c, err := r.EncodeCompany()
	if err != nil {
		return fmt.Errorf("failed to encode company metadata: %w", err)
	}
	if err := w.batch.Set(append([]byte("meta:"), r.CompanyID...), c); err != nil {
		return fmt.Errorf("failed to write company metadata: %w", err)
	}

	p, err := r.EncodePartner()
	if err != nil {
		return fmt.Errorf("failed to encode partner metadata: %w", err)
	}
	if err := w.batch.Set(append([]byte("meta:"), r.PartnerID...), p); err != nil {
		return fmt.Errorf("failed to write partner metadata: %w", err)
	}

	k1 := make([]byte, 6+len(r.CompanyID)+len(r.PartnerID))
	copy(k1[0:4], "rel:")
	copy(k1[4:4+len(r.CompanyID)], r.CompanyID)
	copy(k1[4+len(r.CompanyID):6+len(r.CompanyID)], "->")
	copy(k1[6+len(r.CompanyID):], r.PartnerID)
	if err := w.batch.Set(k1, []byte{}); err != nil {
		return fmt.Errorf("failed to write company relationship: %w", err)
	}

	k2 := make([]byte, 6+len(r.CompanyID)+len(r.PartnerID))
	copy(k2[0:4], "rel:")
	copy(k2[4:4+len(r.PartnerID)], r.PartnerID)
	copy(k2[4+len(r.PartnerID):6+len(r.PartnerID)], "<-")
	copy(k2[6+len(r.PartnerID):], r.CompanyID)
	if err := w.batch.Set(k2, []byte{}); err != nil {
		return fmt.Errorf("failed to write partner relationship: %w", err)
	}
	return nil
}

func NewWriter(pth string) (*Writer, error) {
	if err := os.RemoveAll(pth); err != nil {
		return nil, fmt.Errorf("could not remove existing graph directory: %w", err)
	}
	opts := badger.DefaultOptions(pth).WithLoggingLevel(badger.WARNING)
	kv, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("could not open badger: %w", err)
	}
	b := kv.NewWriteBatch()
	return &Writer{pth, kv, b}, nil
}
