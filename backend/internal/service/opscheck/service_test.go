package opscheck

import (
	"testing"

	"caseagent/internal/config"
)

func TestOverallStatus(t *testing.T) {
	cases := []struct {
		name   string
		checks []Check
		want   string
	}{
		{
			name:   "all pass",
			checks: []Check{{Status: StatusPass}, {Status: StatusPass}},
			want:   StatusPass,
		},
		{
			name:   "warn without fail",
			checks: []Check{{Status: StatusPass}, {Status: StatusWarn}},
			want:   StatusWarn,
		},
		{
			name:   "fail wins",
			checks: []Check{{Status: StatusWarn}, {Status: StatusFail}},
			want:   StatusFail,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OverallStatus(tc.checks); got != tc.want {
				t.Fatalf("OverallStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTableNamesValuesSQLQuotesNames(t *testing.T) {
	got := tableNamesValuesSQL([]string{"projects", "odd'name"})
	want := "('projects'),('odd''name')"
	if got != want {
		t.Fatalf("tableNamesValuesSQL() = %q, want %q", got, want)
	}
}

func TestCheckWorkerRiskClassifiesPassWarnFail(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		check := New(nil, &config.Config{JobRunner: config.JobRunnerConfig{
			MaxConcurrency:            4,
			TenantMaxConcurrency:      2,
			MaxRetries:                2,
			RetryBackoffSeconds:       5,
			PollIntervalSeconds:       2,
			RunningTimeoutSeconds:     900,
			StateUpdateTimeoutSeconds: 10,
		}}).checkWorkerRisk()

		if check.Status != StatusPass {
			t.Fatalf("status = %q, want pass: %#v", check.Status, check)
		}
	})

	t.Run("warn", func(t *testing.T) {
		check := New(nil, &config.Config{JobRunner: config.JobRunnerConfig{
			MaxConcurrency:            2,
			TenantMaxConcurrency:      0,
			MaxRetries:                0,
			RetryBackoffSeconds:       5,
			PollIntervalSeconds:       2,
			RunningTimeoutSeconds:     900,
			StateUpdateTimeoutSeconds: 10,
		}}).checkWorkerRisk()

		if check.Status != StatusWarn {
			t.Fatalf("status = %q, want warn: %#v", check.Status, check)
		}
	})

	t.Run("fail", func(t *testing.T) {
		check := New(nil, &config.Config{JobRunner: config.JobRunnerConfig{
			MaxConcurrency:            0,
			TenantMaxConcurrency:      1,
			MaxRetries:                1,
			RetryBackoffSeconds:       5,
			PollIntervalSeconds:       2,
			RunningTimeoutSeconds:     900,
			StateUpdateTimeoutSeconds: 10,
		}}).checkWorkerRisk()

		if check.Status != StatusFail {
			t.Fatalf("status = %q, want fail: %#v", check.Status, check)
		}
	})
}
