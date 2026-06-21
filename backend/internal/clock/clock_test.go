package clock

import (
	"testing"
	"time"
)

func TestNowUsesUTC(t *testing.T) {
	got := Now()
	if got.Location() != time.UTC {
		t.Fatalf("Now() location = %s, want UTC", got.Location())
	}
}
