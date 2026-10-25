package clock_test

import (
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
)

func TestRealReturnsCurrentUTC(t *testing.T) {
	var c clock.Clock = clock.Real{}
	before := time.Now()
	got := c.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) || got.Location() != time.UTC {
		t.Fatalf("Now()=%v, expected UTC time between %v and %v", got, before, after)
	}
}
