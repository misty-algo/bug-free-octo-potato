package main

import "testing"

func TestSchemaOptionsAreReadOnlyByDefault(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		apply, fail bool
	}{
		{"default", nil, false, false},
		{"explicit apply", []string{"--apply"}, true, false},
		{"unknown flag", []string{"--drop"}, false, true},
		{"trailing argument", []string{"production"}, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseOptions(tt.args)
			if (err != nil) != tt.fail {
				t.Fatalf("error=%v", err)
			}
			if opts.apply != tt.apply {
				t.Fatalf("apply=%v", opts.apply)
			}
		})
	}
}
