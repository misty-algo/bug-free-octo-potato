package handler

import (
	"context"
	"time"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// SafetyServer 暴露安全事件域。
// 身份一律来自令牌，任务引用由服务端核对，位置不接受 0,0 作为证据。
type SafetyServer struct {
	driverv1.UnimplementedSafetyServiceServer
	safety *service.SafetyService
	notice *service.NoticeService
	auth   authn.Config
}

func NewSafetyServer(s *service.SafetyService, n *service.NoticeService, cfg authn.Config) *SafetyServer {
	return &SafetyServer{safety: s, notice: n, auth: cfg}
}

func (s *SafetyServer) ReportEvent(ctx context.Context, r *driverv1.ReportEventRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.Report(ctx, dto.ReportEventCommand{
		Principal: principal, ClientRequestID: r.GetClientRequestId(),
		EventType: r.GetEventType(), Description: r.GetDescription(),
		TaskID: r.GetTaskId(), HasPosition: r.GetHasPosition(),
		Lng: r.GetLng(), Lat: r.GetLat(), CapturedAt: timePtr(r.GetCapturedAt()),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) EventByRequest(ctx context.Context, r *driverv1.EventByRequestRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.ByRequest(ctx, dto.ByRequestCommand{
		Principal: principal, ClientRequestID: r.GetClientRequestId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) AddSupplement(ctx context.Context, r *driverv1.AddSupplementRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.AddSupplement(ctx, dto.AddSupplementCommand{
		Principal: principal, EventID: r.GetEventId(), Kind: r.GetKind(),
		Note: r.GetNote(), HasPosition: r.GetHasPosition(),
		Lng: r.GetLng(), Lat: r.GetLat(), CapturedAt: timePtr(r.GetCapturedAt()),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) AddAttachment(ctx context.Context, r *driverv1.AddAttachmentRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.AddAttachment(ctx, dto.AddAttachmentCommand{
		Principal: principal, EventID: r.GetEventId(), FileID: r.GetFileId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) Hotline(ctx context.Context, _ *driverv1.HotlineRequest) (*driverv1.HotlineView, error) {
	if _, err := principalFrom(ctx, s.auth); err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view := s.safety.Hotline()
	out := &driverv1.HotlineView{
		Version: view.Version, PublishedAt: unix(&view.PublishedAt),
		AutoDial: view.AutoDial, Demo: view.Demo, OfflineCacheable: view.OfflineCacheable,
	}
	for _, item := range view.Items {
		out.Items = append(out.Items, &driverv1.HotlineItem{
			Name: item.Name, Number: item.Number, Note: item.Note,
		})
	}
	return out, nil
}

func (s *SafetyServer) PendingEvents(ctx context.Context, r *driverv1.PendingEventsRequest) (*driverv1.PendingEventsResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	views, err := s.safety.Pending(ctx, dto.PendingEventsCommand{
		Principal: principal, State: r.GetState(), Limit: int(r.GetLimit()), DriverID: r.GetDriverId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	out := &driverv1.PendingEventsResponse{}
	for i := range views {
		out.Items = append(out.Items, toSafetyEvent(&views[i]))
	}
	return out, nil
}

func (s *SafetyServer) DrainSafetyOutbox(ctx context.Context, r *driverv1.DrainSafetyOutboxRequest) (*driverv1.DrainSafetyOutboxResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	delivered, failed, err := s.safety.DrainOutbox(ctx, principal, int(r.GetLimit()))
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.DrainSafetyOutboxResponse{Delivered: delivered, Failed: failed}, nil
}

func (s *SafetyServer) ClaimEvent(ctx context.Context, r *driverv1.ClaimEventRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.Claim(ctx, dto.SafetyClaimCommand{
		Principal: principal, EventID: r.GetEventId(),
		ExpectedVersion: r.GetExpectedVersion(), RequestID: r.GetRequestId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) StartProcessing(ctx context.Context, r *driverv1.StartProcessingRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.Start(ctx, dto.SafetyStartCommand{
		Principal: principal, EventID: r.GetEventId(),
		ExpectedVersion: r.GetExpectedVersion(), RequestID: r.GetRequestId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) TransferEvent(ctx context.Context, r *driverv1.TransferEventRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.Transfer(ctx, dto.SafetyTransferCommand{
		Principal: principal, EventID: r.GetEventId(),
		ExpectedVersion: r.GetExpectedVersion(), To: r.GetTo(), Reason: r.GetReason(), RequestID: r.GetRequestId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) AddAction(ctx context.Context, r *driverv1.AddActionRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.AddAction(ctx, dto.SafetyAddActionCommand{
		Principal: principal, EventID: r.GetEventId(),
		ExpectedVersion: r.GetExpectedVersion(), Kind: r.GetKind(),
		Note: r.GetNote(), RefID: r.GetRefId(), RequestID: r.GetRequestId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) ResolveEvent(ctx context.Context, r *driverv1.ResolveEventRequest) (*driverv1.SafetyEventView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.Resolve(ctx, dto.SafetyResolveCommand{
		Principal: principal, EventID: r.GetEventId(),
		ExpectedVersion: r.GetExpectedVersion(), Result: r.GetResult(),
		Note: r.GetNote(), RequestID: r.GetRequestId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toSafetyEvent(view), nil
}

func (s *SafetyServer) GetEvidence(ctx context.Context, r *driverv1.GetEvidenceRequest) (*driverv1.EvidenceView, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.safety.GetEvidence(ctx, dto.SafetyGetEvidenceCommand{
		Principal: principal, EventID: r.GetEventId(),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toEvidence(view), nil
}

func (s *SafetyServer) EscalateTick(ctx context.Context, r *driverv1.EscalateTickRequest) (*driverv1.EscalateTickResponse, error) {
	principal, err := principalFrom(ctx, s.auth)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	var now time.Time
	if r.GetNow() > 0 {
		now = time.Unix(r.GetNow(), 0).UTC()
	}
	result, err := s.safety.EscalateTick(ctx, dto.SafetyEscalateTickCommand{
		Principal: principal, Now: now, Limit: int(r.GetLimit()),
	})
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return &driverv1.EscalateTickResponse{
		Escalated: result.Escalated, Skipped: result.Skipped, Failed: result.Failed,
	}, nil
}

func (s *SafetyServer) GetNotice(ctx context.Context, r *driverv1.GetNoticeRequest) (*driverv1.NoticeView, error) {
	if _, err := principalFrom(ctx, s.auth); err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	view, err := s.notice.Notice(ctx, r.GetKey())
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	return toNotice(view), nil
}

func (s *SafetyServer) ListNotices(ctx context.Context, _ *driverv1.ListNoticesRequest) (*driverv1.ListNoticesResponse, error) {
	if _, err := principalFrom(ctx, s.auth); err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	items, err := s.notice.Manifest(ctx)
	if err != nil {
		return nil, apperrors.ToGRPC(err)
	}
	out := &driverv1.ListNoticesResponse{}
	for i := range items {
		out.Items = append(out.Items, &driverv1.NoticeItem{
			Key: items[i].Key, Title: items[i].Title, Version: items[i].Version, Etag: items[i].ETag,
		})
	}
	return out, nil
}

func toEvidence(v *dto.EvidenceView) *driverv1.EvidenceView {
	if v == nil {
		return nil
	}
	out := &driverv1.EvidenceView{
		EventId: v.EventID, TaskId: v.TaskID,
		Recording: &driverv1.RecordingView{Available: v.Recording.Available, Note: v.Recording.Note},
	}
	for i := range v.Trace {
		tp := v.Trace[i]
		out.Trace = append(out.Trace, &driverv1.TracePointView{
			Seq: tp.Seq, CollectedAt: tp.CollectedAt.Unix(),
			Lng: tp.Lng, Lat: tp.Lat, Accuracy: tp.Accuracy,
		})
	}
	for i := range v.Messages {
		m := v.Messages[i]
		out.Messages = append(out.Messages, &driverv1.MessageView{
			Seq: m.Seq, From: m.From, Text: m.Text, At: m.At.Unix(),
		})
	}
	if v.VirtualCall != nil {
		out.VirtualCall = &driverv1.VirtualCallView{
			Number: v.VirtualCall.Number, ExpiresAt: v.VirtualCall.ExpiresAt.Unix(),
			Simulated: v.VirtualCall.Simulated,
		}
		if v.VirtualCall.RevokedAt != nil {
			out.VirtualCall.RevokedAt = v.VirtualCall.RevokedAt.Unix()
		}
	}
	return out
}

func toNotice(v *dto.NoticeView) *driverv1.NoticeView {
	out := &driverv1.NoticeView{Key: v.Key, Title: v.Title, Version: v.Version, Etag: v.ETag}
	for _, b := range v.Blocks {
		out.Blocks = append(out.Blocks, &driverv1.NoticeBlockView{Kind: b.Kind, Text: b.Text})
	}
	return out
}

func toSafetyEvent(v *dto.SafetyEventView) *driverv1.SafetyEventView {
	if v == nil {
		return nil
	}
	return &driverv1.SafetyEventView{
		EventId: v.EventID, DriverId: v.DriverID, State: v.State,
		Version: v.Version, Handler: v.Handler, ClaimedAt: unix(v.ClaimedAt),
		EventType: v.EventType, Description: v.Description, TaskId: v.TaskID,
		Position:    toSafetyPosition(v.Position),
		ReceivedAt:  unix(v.ReceivedAt),
		Attachments: v.Attachments,
		Supplements: int32(v.Supplements),
		Actions:     int32(v.Actions), Escalations: int32(v.Escalations),
	}
}

func toSafetyPosition(p entity.SafetyPosition) *driverv1.PositionView {
	lng, lat := 0.0, 0.0
	if p.Lng != nil {
		lng = *p.Lng
	}
	if p.Lat != nil {
		lat = *p.Lat
	}
	return &driverv1.PositionView{
		Kind: p.Kind, Lng: lng, Lat: lat,
		CapturedAt: unix(p.CapturedAt), Source: p.Source,
	}
}
