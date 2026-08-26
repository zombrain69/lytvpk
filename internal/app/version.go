package app

const (
	// CommunityForkExecutableName is the canonical executable shipped by this
	// fork. Release ZIPs must contain this exact file so the updater can reject
	// ambiguous archives.
	CommunityForkExecutableName = "LytVPK-Community-Fork.exe"

	// UpstreamGithubRepo is the original project this community fork is based on.
	// It is attribution only and is never used as an update source.
	UpstreamGithubRepo = "LaoYutang/lytvpk"
)

var (
	// AppVersion is set by main at startup so existing -ldflags targeting
	// main.AppVersion continue to work. Keep this fallback in sync with main
	// for tests and non-Wails callers.
	AppVersion = "2.5.14-community.4"

	// UpdateRepo is this Community Fork's GitHub release repository in
	// "owner/repo" form. It is deliberately separate from the upstream
	// attribution repository and is the only allowed in-app update source.
	UpdateRepo = "zombrain69/lytvpk"
)
