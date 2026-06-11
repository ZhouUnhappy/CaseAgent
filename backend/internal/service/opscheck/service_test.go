package opscheck

import "testing"

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
