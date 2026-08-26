package app

const (
	// UpstreamGithubRepo is the original project this community fork is based on.
	// It is attribution only and is never used as an update source.
	UpstreamGithubRepo = "LaoYutang/lytvpk"
)

var (
	// AppVersion is set by main at startup so existing -ldflags targeting
	// main.AppVersion continue to work.
	AppVersion = "0.0.0-dev"

	// UpdateRepo is this fork's GitHub release repository in "owner/repo" form.
	// It is intentionally empty in source builds: an unconfigured fork must
	// never check for or install updates from the upstream project.
	UpdateRepo string
)
