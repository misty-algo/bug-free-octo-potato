package service

import (
	"testing"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
)

func TestNormalizeRejectItems(t *testing.T) {
	tests := []struct {
		name    string
		items   []dto.RejectItemInput
		wantErr bool
		wantLen int
	}{
		{
			name:    "empty",
			items:   nil,
			wantErr: true,
		},
		{
			name: "missing reason",
			items: []dto.RejectItemInput{
				{TargetType: "doc_type", TargetKey: "insurance", Code: "DOC_EXPIRED"},
			},
			wantErr: true,
		},
		{
			name: "invalid target type",
			items: []dto.RejectItemInput{
				{TargetType: "driver", TargetKey: "x", Code: "DOC_BLURRY", Reason: "blur"},
			},
			wantErr: true,
		},
		{
			name: "normal",
			items: []dto.RejectItemInput{
				{TargetType: "doc_type", TargetKey: "driving_license", Code: "DOC_BLURRY", Reason: "unreadable"},
				{TargetType: "vehicle", TargetKey: "V:abc", Code: "VEHICLE_INFO_MISMATCH", Reason: "plate mismatch"},
			},
			wantLen: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRejectItems(tt.items)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeRejectItems() err=%v, wantErr=%v", err, tt.wantErr)
			}
			if len(got) != tt.wantLen {
				t.Errorf("normalizeRejectItems() len=%d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestSplitRevisionRef(t *testing.T) {
	tests := []struct {
		ref     string
		wantApp string
		wantRev int64
		wantOK  bool
	}{
		{"APP-1:3", "APP-1", 3, true},
		{"APP:1", "APP", 1, true},
		{"APP-1", "", 0, false},
		{"APP-1:", "", 0, false},
		{"APP-1:abc", "", 0, false},
		{"", "", 0, false},
	}
	for _, tt := range tests {
		app, rev, ok := splitRevisionRef(tt.ref)
		if ok != tt.wantOK || app != tt.wantApp || rev != tt.wantRev {
			t.Errorf("splitRevisionRef(%q)=(%q,%d,%v), want (%q,%d,%v)",
				tt.ref, app, rev, ok, tt.wantApp, tt.wantRev, tt.wantOK)
		}
	}
}
