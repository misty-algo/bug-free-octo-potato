package request

import (
	"strings"
	"testing"
)

func TestDecodeArrivalJSON(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		ok         bool
	}{
		{"valid", `{"version":"1","nested":{"x":2}}`, true},
		{"duplicate", `{"version":"1","version":"2"}`, false},
		{"nested duplicate", `{"nested":{"x":1,"x":2}}`, false},
		{"escaped duplicate", `{"version":"1","\u0076ersion":"2"}`, false},
		{"case insensitive duplicate", `{"version":"1","Version":"2"}`, false},
		{"multiple documents", `{} {}`, false},
		{"unknown", `{"other":1}`, false},
		{"null", `null`, false},
		{"oversized", strings.Repeat(" ", 16385) + `{}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out struct {
				Version string         `json:"version"`
				Nested  map[string]int `json:"nested"`
			}
			err := DecodeArrivalJSON(strings.NewReader(tt.body), &out)
			if (err == nil) != tt.ok {
				t.Fatalf("err=%v want valid=%v", err, tt.ok)
			}
		})
	}
}

func TestDecimalVersion(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  uint64
		ok    bool
	}{
		{"0", 0, true}, {"18446744073709551615", ^uint64(0), true}, {"", 0, false}, {"-1", 0, false}, {"+1", 0, false}, {"1.0", 0, false}, {"18446744073709551616", 0, false}, {" 1", 0, false},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err == nil) != tt.ok || got != tt.want {
				t.Fatalf("got %d %v", got, err)
			}
		})
	}
}

func TestIdempotencyKey(t *testing.T) {
	for _, tt := range []struct {
		key string
		ok  bool
	}{{"abc-123", true}, {strings.Repeat("a", 128), true}, {"", false}, {strings.Repeat("a", 129), false}, {"键", false}, {"a\nb", false}, {" ", false}} {
		if err := ValidateIdempotencyKey(tt.key); (err == nil) != tt.ok {
			t.Errorf("%q err=%v", tt.key, err)
		}
	}
}
