package telemetry

import (
	"testing"

	"github.com/fumoid/x-course-avito/internal/config"
)

func TestSamplerFromEnv(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  config.Config
	}{
		{name: "always_on", cfg: config.Config{OTELTracesSampler: "always_on"}},
		{name: "always_off", cfg: config.Config{OTELTracesSampler: "always_off"}},
		{name: "traceidratio", cfg: config.Config{OTELTracesSampler: "traceidratio", OTELTracesSamplerArg: "0.1"}},
		{name: "parentbased_always_off", cfg: config.Config{OTELTracesSampler: "parentbased_always_off"}},
		{name: "default", cfg: config.Config{OTELTracesSampler: "parentbased_always_on"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if samplerFromEnv(tc.cfg) == nil {
				t.Fatal("sampler is nil")
			}
		})
	}
}

func TestParseRatio(t *testing.T) {
	t.Parallel()
	if parseRatio("") != 1 {
		t.Fatal("empty")
	}
	if parseRatio("0.5") != 0.5 {
		t.Fatal("0.5")
	}
	if parseRatio("2") != 1 {
		t.Fatal("clamp")
	}
	if parseRatio("nope") != 1 {
		t.Fatal("invalid")
	}
}
