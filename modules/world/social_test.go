package world

import "testing"

// TestReasonLabelAllValid pins every in-range ReportAbuseReason value
// to its canonical string label. Mirrors TS ReportAbuse.ts:4-17.
func TestReasonLabelAllValid(t *testing.T) {
	cases := []struct {
		reason ReportAbuseReason
		want   string
	}{
		{ReportAbuseOffensiveLanguage, "OFFENSIVE_LANGUAGE"},
		{ReportAbuseItemScamming, "ITEM_SCAMMING"},
		{ReportAbusePasswordScamming, "PASSWORD_SCAMMING"},
		{ReportAbuseBugAbuse, "BUG_ABUSE"},
		{ReportAbuseStaffImpersonation, "STAFF_IMPERSONATION"},
		{ReportAbuseAccountSharing, "ACCOUNT_SHARING"},
		{ReportAbuseMacroing, "MACROING"},
		{ReportAbuseMultiLogging, "MULTI_LOGGING"},
		{ReportAbuseEncouragingBreakRules, "ENCOURAGING_BREAK_RULES"},
		{ReportAbuseMisuseCustomerSupport, "MISUSE_CUSTOMER_SUPPORT"},
		{ReportAbuseAdvertisingWebsite, "ADVERTISING_WEBSITE"},
		{ReportAbuseRealWorldTrading, "REAL_WORLD_TRADING"},
	}
	for _, tc := range cases {
		if got := reasonLabel(tc.reason); got != tc.want {
			t.Errorf("reasonLabel(%d): got %q, want %q", tc.reason, got, tc.want)
		}
	}
}

// TestReasonLabelOutOfRangeReturnsEmpty pins out-of-range behavior.
func TestReasonLabelOutOfRangeReturnsEmpty(t *testing.T) {
	for _, r := range []ReportAbuseReason{12, 13, 100, 255} {
		if got := reasonLabel(r); got != "" {
			t.Errorf("reasonLabel(%d): got %q, want \"\"", r, got)
		}
	}
}

// TestReportAbuseReasonRangeBoundary pins the constants used by the
// handler's range gate at handler_reportabuse.go.
func TestReportAbuseReasonRangeBoundary(t *testing.T) {
	if ReportAbuseOffensiveLanguage != 0 {
		t.Errorf("ReportAbuseOffensiveLanguage: got %d, want 0", ReportAbuseOffensiveLanguage)
	}
	if ReportAbuseRealWorldTrading != 11 {
		t.Errorf("ReportAbuseRealWorldTrading: got %d, want 11", ReportAbuseRealWorldTrading)
	}
}
