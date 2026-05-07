package session

import "runtime"

// bridgeAppVersion is the version Proton's API recognizes for the Bridge
// product. Empirically tested 2026-04-26: the live `/auth/v4/info` endpoint
// rejects every other product name we tried (`mail`, `account`, `Other`,
// `protonmail-mcp`, `mcp`) with code 2064. Only `bridge` passes the product
// allowlist.
//
// We pin to the latest published proton-bridge release rather than a
// fictional protonmail-mcp version because Proton's API gates accepted
// versions per product (HTTP 5002/5003 once minimums advance). When live
// auth starts rejecting this with code 5002 ("Invalid app version") or 5003
// ("AppVersionBad"), bump to whatever proton-bridge has tagged latest:
//
//	gh api repos/ProtonMail/proton-bridge/releases/latest -q .tag_name
const bridgeAppVersion = "3.24.1"

// appVersionHeader returns a Proton-acceptable x-pm-appversion header value.
// Format follows proton-bridge: <api-os>-bridge@<version>, all lowercase.
//
// We send Bridge's identity (rather than identifying as protonmail-mcp)
// because Proton's API rejects unknown product names with code 2064. The
// macos/linux/windows platform prefix is mapped from runtime.GOOS the same
// way proton-bridge does.
func appVersionHeader() string {
	return apiOS() + "-bridge@" + bridgeAppVersion
}

func apiOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		// linux + anything else falls back to linux. Matches proton-bridge.
		return "linux"
	}
}
