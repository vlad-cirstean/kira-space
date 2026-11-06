package startupfail

// Info is the app-identifying seam Classify/RenderAlert/RenderClipboard need but this package
// itself must not: hoisted to repo-root internal/, startupfail can no longer import
// either app's own internal/config (Go's internal/ visibility rule — a repo-root package cannot
// import anything under apps/kira-space/internal or apps/kira-space/internal) or an
// internal/buildinfo (which stays per-app, since it's genuinely just a one-line Version var with no
// shared logic worth hoisting). Every caller supplies its own Info instead of this package reaching
// out for one.
type Info struct {
	// AppName is the display name every rendered headline/advice string names itself by ("Kira
	// Studio" / "Kira Space") — previously hardcoded here, now supplied per call.
	AppName string
	// Version is this build's version string — previously buildinfo.Version, read directly.
	Version string
	// Home, LogsDir and DbPath mirror the three internal/config reads Classify/RenderAlert/
	// RenderClipboard used to make directly (KiraHome/LogsDir/DbPath). Each defaults to a function
	// returning "" when left nil (NewReporter), so a zero-value Info never panics — it renders
	// an empty path, same as an empty string literal would have.
	Home    func() string
	LogsDir func() string
	DbPath  func() string
}

func (i Info) home() string {
	if i.Home == nil {
		return ""
	}
	return i.Home()
}

func (i Info) logsDir() string {
	if i.LogsDir == nil {
		return ""
	}
	return i.LogsDir()
}

func (i Info) dbPath() string {
	if i.DbPath == nil {
		return ""
	}
	return i.DbPath()
}
