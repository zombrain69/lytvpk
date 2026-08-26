package app

import "testing"

func TestIsDevelopmentBuildVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "exact development version", version: "0.0.0-dev", want: true},
		{name: "development version with whitespace", version: " 0.0.0-dev ", want: true},
		{name: "community release", version: "2.5.14-community.2", want: false},
		{name: "CI build", version: "0.0.0-ci", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isDevelopmentBuildVersion(test.version); got != test.want {
				t.Fatalf("isDevelopmentBuildVersion(%q) = %v, want %v", test.version, got, test.want)
			}
		})
	}
}

func TestCheckUpdateSkipsDevelopmentBuild(t *testing.T) {
	originalVersion := AppVersion
	t.Cleanup(func() { AppVersion = originalVersion })
	AppVersion = developmentBuildVersion

	info := (&App{}).CheckUpdate()
	if info.HasUpdate {
		t.Fatal("development build must not report an update")
	}
	if info.CurrentVer != developmentBuildVersion {
		t.Fatalf("CurrentVer = %q, want %q", info.CurrentVer, developmentBuildVersion)
	}
	if info.Error == "" {
		t.Fatal("development build must explain why update checking was skipped")
	}
}
