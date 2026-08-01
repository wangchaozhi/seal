package auth

// Principal 由真实登录中间件注入。骨架阶段不接收前端伪造的 paid/vip 字段。
type Principal struct {
	UserID        string
	Authenticated bool
	VIP           bool
	PaidExport    bool
}

type ExportPolicy struct {
	AddWatermark bool
	MaxSize      int
	AllowSVG     bool
}

func CalculateExportPolicy(principal Principal, usesPremiumFeature bool) ExportPolicy {
	unlocked := principal.Authenticated && (principal.VIP || principal.PaidExport)
	if usesPremiumFeature && !unlocked {
		return ExportPolicy{AddWatermark: true, MaxSize: 1200, AllowSVG: false}
	}
	if unlocked {
		return ExportPolicy{AddWatermark: false, MaxSize: 5000, AllowSVG: true}
	}
	return ExportPolicy{AddWatermark: false, MaxSize: 2000, AllowSVG: true}
}
