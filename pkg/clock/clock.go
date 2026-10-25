// Package clock supplies an injectable time source for business decisions.
package clock

import "time"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// Real uses the system clock and returns UTC timestamps.
type Real struct{}

func (Real) Now() time.Time {
	return time.Now().UTC()
}
