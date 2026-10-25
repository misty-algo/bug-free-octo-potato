package entity

import "testing"

func TestPayloadDigest(t *testing.T) {
	base := RevisionPayload{
		VehicleID: "V:abc", VinMask: "LSV**********0001", Plate: "SHAD15T1",
		CityCode: "shanghai", ModelCode: "sedan", Seats: 4, Color: "white",
		DocumentIDs: []string{"DOC:1", "DOC:2"}, FileIDs: []string{"F:1"},
		DraftID: "VD:1", DraftVersion: 1,
	}
	digest := PayloadDigest(base)
	if digest == "" || len(digest) != 32 {
		t.Fatalf("PayloadDigest()=%q, want 32 hex chars", digest)
	}
	if again := PayloadDigest(base); again != digest {
		t.Errorf("PayloadDigest() is not deterministic: %q vs %q", digest, again)
	}

	changed := base
	changed.Color = "black"
	if PayloadDigest(changed) == digest {
		t.Error("PayloadDigest() must change when frozen payload changes")
	}

	fileChanged := base
	fileChanged.FileIDs = []string{"F:2"}
	if PayloadDigest(fileChanged) == digest {
		t.Error("PayloadDigest() must change when attachment set changes")
	}
}

func TestApplicationRevisionLookup(t *testing.T) {
	app := &VehicleApplication{
		ActiveRevision: 2,
		Revisions: []ApplicationRevision{
			{Revision: 1, State: AppStateRejected},
			{Revision: 2, State: AppStateApproved},
		},
	}
	if rev := app.ActiveRevisionView(); rev == nil || rev.Revision != 2 {
		t.Fatalf("ActiveRevisionView()=%v, want revision 2", rev)
	}
	if rev := app.RevisionByNumber(1); rev == nil || rev.State != AppStateRejected {
		t.Errorf("rejected revision must stay readable, got %v", rev)
	}
	if rev := app.RevisionByNumber(9); rev != nil {
		t.Errorf("RevisionByNumber(9)=%v, want nil", rev)
	}
	if seq := app.NextAuditSeq(); seq != 1 {
		t.Errorf("NextAuditSeq()=%d, want 1 for empty audit trail", seq)
	}
}

func TestApplicationIsActive(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{AppStatePending, true},
		{AppStateReviewing, true},
		{AppStateApproved, false},
		{AppStateRejected, false},
		{AppStateWithdrawn, false},
	}
	for _, tt := range tests {
		app := &VehicleApplication{State: tt.state}
		if got := app.IsActive(); got != tt.want {
			t.Errorf("IsActive() with state %q = %v, want %v", tt.state, got, tt.want)
		}
	}
}
