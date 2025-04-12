package finalize

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestFindLastTarget(t *testing.T) {
	tests := []struct {
		name      string
		rrs       []dns.RR
		qname     string
		want      string
		expectErr bool
	}{
		{
			name: "single CNAME",
			rrs: []dns.RR{
				&dns.CNAME{Hdr: dns.RR_Header{Name: "a.example.com.", Rrtype: dns.TypeCNAME}, Target: "b.example.com."},
			},
			qname:     "a.example.com.",
			want:      "b.example.com.",
			expectErr: false,
		},
		{
			name: "valid CNAME chain",
			rrs: []dns.RR{
				&dns.CNAME{Hdr: dns.RR_Header{Name: "a.example.com.", Rrtype: dns.TypeCNAME}, Target: "b.example.com."},
				&dns.CNAME{Hdr: dns.RR_Header{Name: "b.example.com.", Rrtype: dns.TypeCNAME}, Target: "c.example.com."},
			},
			qname:     "a.example.com.",
			want:      "c.example.com.",
			expectErr: false,
		},
		{
			name: "no CNAME records for qname",
			rrs: []dns.RR{
				&dns.CNAME{Hdr: dns.RR_Header{Name: "b.example.com.", Rrtype: dns.TypeCNAME}, Target: "c.example.com."},
			},
			qname:     "a.example.com.",
			want:      "",
			expectErr: true,
		},
		{
			name: "circular reference",
			rrs: []dns.RR{
				&dns.CNAME{Hdr: dns.RR_Header{Name: "a.example.com.", Rrtype: dns.TypeCNAME}, Target: "b.example.com."},
				&dns.CNAME{Hdr: dns.RR_Header{Name: "b.example.com.", Rrtype: dns.TypeCNAME}, Target: "a.example.com."},
			},
			qname:     "a.example.com.",
			want:      "",
			expectErr: true,
		},
		{
			name: "no CNAME records",
			rrs: []dns.RR{
				&dns.A{Hdr: dns.RR_Header{Name: "a.example.com.", Rrtype: dns.TypeA}, A: net.IP{1, 2, 3, 4}},
			},
			qname:     "a.example.com.",
			want:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findLastTarget(tt.rrs, tt.qname)
			if (err != nil) != tt.expectErr {
				t.Errorf("findLastTarget() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if got != tt.want {
				t.Errorf("findLastTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}
