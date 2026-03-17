package probe

import (
	"testing"
)

func TestParseTimedatectl(t *testing.T) {
	input := `Timezone=Africa/Johannesburg
NTPSynchronized=yes
TimeUSec=Mon 2026-03-17 14:22:00 SAST
RTC=no`

	fields := parseTimedatectl(input)
	if fields["NTPSynchronized"] != "yes" {
		t.Errorf("NTPSynchronized = %q, want yes", fields["NTPSynchronized"])
	}
	if fields["Timezone"] != "Africa/Johannesburg" {
		t.Errorf("Timezone = %q, want Africa/Johannesburg", fields["Timezone"])
	}
}
