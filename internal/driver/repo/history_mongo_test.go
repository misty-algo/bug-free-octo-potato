package repo

import (
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

func TestMergePassengerReviews(t *testing.T) {
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		docs       []passengerReviewLite
		ratings    map[string]*entity.HistoryRating
		complaints map[string]*entity.HistoryComplaint
		wantRating int32
		wantText   string
		wantNotice string
	}{
		{
			name:       "普通评价补齐评分与内容",
			docs:       []passengerReviewLite{{OrderNo: "O1", Rating: 5, Comment: "服务很好", CreatedAt: at}},
			ratings:    map[string]*entity.HistoryRating{},
			complaints: map[string]*entity.HistoryComplaint{},
			wantRating: 5,
			wantText:   "服务很好",
		},
		{
			name: "投诉评价补齐投诉通知",
			docs: []passengerReviewLite{{
				OrderNo: "O2", Rating: 1, IsComplaint: true,
				ComplaintType: "服务态度", ComplaintDesc: "司机绕路", CreatedAt: at,
			}},
			ratings:    map[string]*entity.HistoryRating{},
			complaints: map[string]*entity.HistoryComplaint{},
			wantRating: 1,
			wantNotice: "司机绕路",
		},
		{
			name:       "订单域已有评分时不覆盖",
			docs:       []passengerReviewLite{{OrderNo: "O3", Rating: 1, Comment: "新评价", CreatedAt: at}},
			ratings:    map[string]*entity.HistoryRating{"O3": {Score: 5, Content: "旧评分", RatedAt: at}},
			complaints: map[string]*entity.HistoryComplaint{},
			wantRating: 5,
			wantText:   "旧评分",
		},
		{
			name: "工单已有投诉时不覆盖",
			docs: []passengerReviewLite{{
				OrderNo: "O4", IsComplaint: true, ComplaintDesc: "新投诉", CreatedAt: at,
			}},
			ratings:    map[string]*entity.HistoryRating{},
			complaints: map[string]*entity.HistoryComplaint{"O4": {Content: "旧投诉", At: at}},
			wantNotice: "旧投诉",
		},
		{
			name:       "空订单号忽略",
			docs:       []passengerReviewLite{{OrderNo: "", Rating: 5, CreatedAt: at}},
			ratings:    map[string]*entity.HistoryRating{},
			complaints: map[string]*entity.HistoryComplaint{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mergePassengerReviews(tc.docs, tc.ratings, tc.complaints)
			if tc.wantRating == 0 && tc.wantNotice == "" {
				if len(tc.ratings) != 0 || len(tc.complaints) != 0 {
					t.Fatalf("got ratings=%v complaints=%v, want both empty", tc.ratings, tc.complaints)
				}
				return
			}
			if tc.wantRating != 0 {
				got := tc.ratings[tc.docs[0].OrderNo]
				if got == nil {
					t.Fatalf("got rating nil, want score %d", tc.wantRating)
				}
				if got.Score != tc.wantRating || got.Content != tc.wantText {
					t.Fatalf("got score=%d content=%q, want score=%d content=%q",
						got.Score, got.Content, tc.wantRating, tc.wantText)
				}
				if !got.RatedAt.Equal(at) {
					t.Fatalf("got ratedAt=%s, want %s", got.RatedAt, at)
				}
			}
			if tc.wantNotice != "" {
				got := tc.complaints[tc.docs[0].OrderNo]
				if got == nil {
					t.Fatalf("got complaint nil, want content %q", tc.wantNotice)
				}
				if got.Content != tc.wantNotice {
					t.Fatalf("got complaint content=%q, want %q", got.Content, tc.wantNotice)
				}
				if !got.At.Equal(at) {
					t.Fatalf("got complaint at=%s, want %s", got.At, at)
				}
			}
		})
	}
}
