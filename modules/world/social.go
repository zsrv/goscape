package world

// ReportAbuseReason mirrors TS ReportAbuse.ts:4-17. Values 0-11 are
// sent over the wire by REPORT_ABUSE (opcode 190); out-of-range values
// trigger an automated ban (per ReportAbuseHandler.ts:14).
type ReportAbuseReason uint8

const (
	ReportAbuseOffensiveLanguage     ReportAbuseReason = 0
	ReportAbuseItemScamming          ReportAbuseReason = 1
	ReportAbusePasswordScamming      ReportAbuseReason = 2
	ReportAbuseBugAbuse              ReportAbuseReason = 3
	ReportAbuseStaffImpersonation    ReportAbuseReason = 4
	ReportAbuseAccountSharing        ReportAbuseReason = 5
	ReportAbuseMacroing              ReportAbuseReason = 6
	ReportAbuseMultiLogging          ReportAbuseReason = 7
	ReportAbuseEncouragingBreakRules ReportAbuseReason = 8
	ReportAbuseMisuseCustomerSupport ReportAbuseReason = 9
	ReportAbuseAdvertisingWebsite    ReportAbuseReason = 10
	ReportAbuseRealWorldTrading      ReportAbuseReason = 11
)

// reasonLabel returns the canonical string label for a ReportAbuseReason
// value, used as the LoggerBridge.NotifyPlayerReport `reason` argument.
// Out-of-range values return "" (caller is responsible for range-checking
// before calling, per the ReportAbuse handler's gate-then-call order).
func reasonLabel(r ReportAbuseReason) string {
	switch r {
	case ReportAbuseOffensiveLanguage:
		return "OFFENSIVE_LANGUAGE"
	case ReportAbuseItemScamming:
		return "ITEM_SCAMMING"
	case ReportAbusePasswordScamming:
		return "PASSWORD_SCAMMING"
	case ReportAbuseBugAbuse:
		return "BUG_ABUSE"
	case ReportAbuseStaffImpersonation:
		return "STAFF_IMPERSONATION"
	case ReportAbuseAccountSharing:
		return "ACCOUNT_SHARING"
	case ReportAbuseMacroing:
		return "MACROING"
	case ReportAbuseMultiLogging:
		return "MULTI_LOGGING"
	case ReportAbuseEncouragingBreakRules:
		return "ENCOURAGING_BREAK_RULES"
	case ReportAbuseMisuseCustomerSupport:
		return "MISUSE_CUSTOMER_SUPPORT"
	case ReportAbuseAdvertisingWebsite:
		return "ADVERTISING_WEBSITE"
	case ReportAbuseRealWorldTrading:
		return "REAL_WORLD_TRADING"
	}
	return ""
}
