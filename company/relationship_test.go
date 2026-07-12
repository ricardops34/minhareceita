package company

import "testing"

func TestRelationshipSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rel       Relationship
		isCompany bool
		expected  Relationship
	}{
		{
			name: "Company Metadata ignores partner fields",
			rel: Relationship{
				CompanyName: "ACME CORP",
				PartnerCPF:  "12345678901",
				PartnerType: 2,
			},
			isCompany: true,
			expected: Relationship{
				CompanyName: "ACME CORP",
			},
		},
		{
			name: "Physical Person Partner (Type 2) preserves CPF",
			rel: Relationship{
				PartnerName: "JOHN DOE",
				PartnerCPF:  "***123456**",
				PartnerType: 2,
			},
			isCompany: false,
			expected: Relationship{
				PartnerName: "JOHN DOE",
				PartnerCPF:  "***123456**",
				PartnerType: 2,
			},
		},
		{
			name: "Legal Entity Partner (Type 1) ignores CPF",
			rel: Relationship{
				PartnerName: "PARTNER CORP",
				PartnerCPF:  "12345678000100",
				PartnerType: 1,
			},
			isCompany: false,
			expected: Relationship{
				PartnerName: "PARTNER CORP",
				PartnerCPF:  "",
				PartnerType: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var b []byte
			var err error
			if tt.isCompany {
				b, err = tt.rel.EncodeCompany()
			} else {
				b, err = tt.rel.EncodePartner()
			}
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			var decoded Relationship
			err = decoded.Decode(b)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			if tt.isCompany {
				if decoded.CompanyName != tt.expected.CompanyName {
					t.Errorf("decoded CompanyName = %q, expected %q", decoded.CompanyName, tt.expected.CompanyName)
				}
			} else {
				if decoded.PartnerType != tt.expected.PartnerType {
					t.Errorf("decoded PartnerType = %d, expected %d", decoded.PartnerType, tt.expected.PartnerType)
				}
				if decoded.PartnerCPF != tt.expected.PartnerCPF {
					t.Errorf("decoded PartnerCPF = %q, expected %q", decoded.PartnerCPF, tt.expected.PartnerCPF)
				}
				if decoded.PartnerName != tt.expected.PartnerName {
					t.Errorf("decoded PartnerName = %q, expected %q", decoded.PartnerName, tt.expected.PartnerName)
				}
			}
		})
	}
}

func TestRelationshipDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", []byte{}},
		{"physical person payload too short", []byte{2, '1', '2', '3'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var decoded Relationship
			if err := decoded.Decode(tt.payload); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
