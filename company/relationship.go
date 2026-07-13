package company

import (
	"crypto/md5"
	"fmt"
	"log/slog"
)

// Relationship represents a connection between a company and a partner.
type Relationship struct {
	CompanyID   string `json:"cnpj"`
	CompanyName string `json:"razao_social"`
	PartnerID   string `json:"id"`
	PartnerName string `json:"nome"`
	PartnerCPF  string `json:"cpf,omitempty"`
	PartnerType int    `json:"-"`
}

// Custom Binary Layout for Relationship:
//
// +------------------+-----------------------------+-------------------------+
// | Type (1 byte)    | CPF (11 bytes, optional)    | Name (remaining bytes)  |
// +------------------+-----------------------------+-------------------------+
//
// - Type: 1 byte representing the partner type
//   (0: Company, 1: company partner, 2: person partner , 3: foreign partner).
// - CPF: 11 bytes of masked CPF string. Only encoded if Type == 2.
// - Name: The rest of the payload representing the entity's name (string).

// EncodeCompany serializes the metadata of the company endpoint into our zero-allocation custom binary format.
func (r *Relationship) EncodeCompany() ([]byte, error) {
	b := make([]byte, 1+len(r.CompanyName))
	b[0] = 0
	copy(b[1:], r.CompanyName)
	return b, nil
}

// EncodePartner serializes the metadata of the partner endpoint into our zero-allocation custom binary format.
func (r *Relationship) EncodePartner() ([]byte, error) {
	n := r.PartnerName
	c := r.PartnerCPF
	t := r.PartnerType

	if t != 2 {
		c = ""
	}

	b := make([]byte, 1+len(c)+len(n))
	b[0] = byte(t)
	copy(b[1:1+len(c)], c)
	copy(b[1+len(c):], n)
	return b, nil
}

// Decode deserializes the metadata of the relationship entity from our custom binary format.
// It detects the entity type based on the first byte (0 for company, 1-3 for partner types) and populates the fields accordingly.
func (r *Relationship) Decode(b []byte) error {
	if len(b) < 1 {
		return fmt.Errorf("metadata payload too short: %d bytes", len(b))
	}
	t := int(b[0])
	if t == 0 {
		r.CompanyName = string(b[1:])
	} else {
		r.PartnerType = t
		if t == 2 {
			if len(b) < 12 {
				return fmt.Errorf("metadata payload corrupt: expected at least 12 bytes for physical person, got %d", len(b))
			}
			r.PartnerCPF = string(b[1:12])
			r.PartnerName = string(b[12:])
		} else {
			r.PartnerCPF = ""
			r.PartnerName = string(b[1:])
		}
	}
	return nil
}

// Relationships streams to a given channel.
func (c *Company) Relationships(ch chan<- *Relationship) {
	for _, p := range c.QuadroSocietario {
		r := Relationship{
			CompanyID:   c.CNPJ,
			CompanyName: c.RazaoSocial,
			PartnerName: p.NomeSocio,
			PartnerType: *p.IdentificadorDeSocio,
		}
		switch r.PartnerType {
		case 1: // legal entity: partner ID is the CNPJ
			r.PartnerID = p.CNPJCPFDoSocio
		case 2: // create hash for person and fill in CPF
			s := fmt.Sprintf("%s%s", p.CNPJCPFDoSocio, p.NomeSocio)
			r.PartnerID = fmt.Sprintf("%x", md5.Sum([]byte(s)))
			r.PartnerCPF = p.CNPJCPFDoSocio
		case 3: // create hash for international partner
			var s string
			if p.CodigoPais != nil {
				s = fmt.Sprintf("%d%s", *p.CodigoPais, p.NomeSocio)
			} else {
				s = p.NomeSocio
			}
			r.PartnerID = fmt.Sprintf("%x", md5.Sum([]byte(s)))
		default:
			slog.Warn("unknown partner type", "value", r.PartnerType)
			continue
		}
		ch <- &r
	}
}
