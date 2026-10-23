package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
)

type fakeEligibilitySources struct {
	docs    []entity.DriverDocument
	binding *entity.VehicleBinding
}

func (f *fakeEligibilitySources) DocumentsBySubject(context.Context, string) ([]entity.DriverDocument, error) {
	return f.docs, nil
}

func (f *fakeEligibilitySources) ActiveBindingByDriver(context.Context, string) (*entity.VehicleBinding, error) {
	return f.binding, nil
}

type fakeAdmission struct{ version int64 }

func (f *fakeAdmission) ByDriver(context.Context, string) (*entity.DriverAdmission, error) {
	return &entity.DriverAdmission{DriverID: "D1", AdmissionVersion: f.version}, nil
}
func (f *fakeAdmission) TryLock(context.Context, string, string, time.Time) (bool, error) {
	return true, nil
}
func (f *fakeAdmission) Unlock(context.Context, string, string) (bool, error) { return true, nil }
func (f *fakeAdmission) BumpVersion(context.Context, string, time.Time) (bool, error) {
	return true, nil
}
func (f *fakeAdmission) SetActiveTask(context.Context, string, string, time.Time) (bool, error) {
	return true, nil
}

func allDocs(expiry time.Time) []entity.DriverDocument {
	out := make([]entity.DriverDocument, 0, len(requiredDocTypes))
	for _, docType := range requiredDocTypes {
		doc := *testDoc("D1", 1, expiry)
		doc.DocType = docType
		doc.DocumentID = "DOC:" + docType
		out = append(out, doc)
	}
	return out
}

func activeBinding() *entity.VehicleBinding {
	return &entity.VehicleBinding{DriverID: "D1", VehicleID: "V1", State: entity.BindingStateActive}
}

func newEligibilityService(docs []entity.DriverDocument, binding *entity.VehicleBinding, version int64) *EligibilityService {
	return NewEligibilityService(
		&fakeEligibilitySources{docs: docs, binding: binding},
		nil, &fakeAdmission{version: version}, nil, clock.Real{}, zap.NewNop(),
	)
}

// TestCheckAllowsWhenEverythingValid 校验全量条件满足才允许接新单。
func TestCheckAllowsWhenEverythingValid(t *testing.T) {
	expiry, err := entity.ExpiresAtFromDate("2027-11-20")
	require.NoError(t, err)
	svc := newEligibilityService(allDocs(expiry), activeBinding(), 7)
	view, err := svc.Check(context.Background(), "D1")
	require.NoError(t, err)
	require.True(t, view.CanAcceptNewTask)
	require.Empty(t, view.Restrictions)
	require.Equal(t, int64(7), view.SourceRevision)
	require.NotNil(t, view.ValidUntil)
	require.Equal(t, expiry.Unix(), view.ValidUntil.Unix(), "valid_until 不晚于最近证件失效时刻")
	require.Equal(t, []string{entity.ActionAcceptTask, entity.ActionGoOnline}, view.AllowedActions)
	require.Empty(t, view.BlockedActions)
}

// TestCheckReportsEveryRestriction 校验单证过期不能被其他证件覆盖，且原因可读。
func TestCheckReportsEveryRestriction(t *testing.T) {
	valid, err := entity.ExpiresAtFromDate("2027-11-20")
	require.NoError(t, err)
	stale, err := entity.ExpiresAtFromDate("2020-01-01")
	require.NoError(t, err)
	docs := allDocs(valid)
	docs[0].ExpiresAt = &stale
	docs[0].ExpiresOn = entity.LastValidDate(stale)

	svc := newEligibilityService(docs, activeBinding(), 7)
	view, err := svc.Check(context.Background(), "D1")
	require.NoError(t, err)
	require.False(t, view.CanAcceptNewTask)
	require.Len(t, view.Restrictions, 1)
	require.Equal(t, entity.RestrictionDocExpired, view.Restrictions[0].Code)
	require.Equal(t, docs[0].DocType, view.Restrictions[0].TargetKey)
	require.Equal(t, entity.SourceDocType, view.Restrictions[0].Source)
	require.Empty(t, view.AllowedActions)
	require.Equal(t, []string{entity.ActionAcceptTask, entity.ActionGoOnline}, view.BlockedActions)
	// 其余三张仍有效，但 valid_until 取最近的失效时刻。
	require.Equal(t, valid.Unix(), view.ValidUntil.Unix())
}

// TestCheckReportsMissingDocAndBinding 校验缺失项逐项列出，而不是笼统拒绝。
func TestCheckReportsMissingDocAndBinding(t *testing.T) {
	expiry, err := entity.ExpiresAtFromDate("2027-11-20")
	require.NoError(t, err)
	svc := newEligibilityService(allDocs(expiry)[1:], nil, 3)
	view, err := svc.Check(context.Background(), "D1")
	require.NoError(t, err)
	require.False(t, view.CanAcceptNewTask)
	require.Len(t, view.Restrictions, 2)
	codes := map[string]bool{}
	for _, r := range view.Restrictions {
		codes[r.Code] = true
	}
	require.True(t, codes[entity.RestrictionDocMissing])
	require.True(t, codes[entity.RestrictionBinding])
	// 缺失项不影响其余证件的有效区间。
	require.Equal(t, expiry.Unix(), view.ValidUntil.Unix())
}
