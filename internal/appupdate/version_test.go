package appupdate

import "testing"

func TestIsReleaseBuild(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"0.0.0", false},
		{"0.0.0-dev", false},
		{"0.0.0-unknown", false},
		{"1.3.0", true},
		{"v1.3.0", true},
		{"1.3.0-rc.1", true},
		{"not-a-version", false},
	}
	for _, tc := range cases {
		if got := isReleaseBuild(tc.version); got != tc.want {
			t.Errorf("isReleaseBuild(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	if got, want := normalize("1.3.0"), normalize("v1.3.0"); got != want {
		t.Errorf("normalize(%q) = %q, normalize(%q) = %q, want equal", "1.3.0", got, "v1.3.0", want)
	}
	if got, want := normalize("1.3.0"), "v1.3.0"; got != want {
		t.Errorf("normalize(%q) = %q, want %q", "1.3.0", got, want)
	}
}
