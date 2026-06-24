package db

import "fmt"

// Relationship represents a connection between a company and a partner.
type Relationship struct {
	CompanyID   string `json:"cnpj" bson:"company_id"`
	CompanyName string `json:"razao_social" bson:"company_name"`
	PartnerID   string `json:"id" bson:"partner_id"`
	PartnerName string `json:"nome" bson:"partner_name"`
	PartnerCPF  string `json:"cpf,omitempty" bson:"partner_cnpf"`
	PartnerType int    `json:"-" bson:"partner_type"`
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
