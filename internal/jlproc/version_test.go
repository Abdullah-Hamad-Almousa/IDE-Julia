package jlproc

import (
	"errors"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		text string
		want Version
	}{
		{text: "1.10.3", want: Version{Major: 1, Minor: 10, Patch: 3}},
		{text: "1.10", want: Version{Major: 1, Minor: 10, Patch: 0}},
		{text: "1.11.0-rc1", want: Version{Major: 1, Minor: 11, Patch: 0, Pre: "rc1"}},
		{text: "1.10.3+build.7", want: Version{Major: 1, Minor: 10, Patch: 3}},
		{text: " 1.10.3 ", want: Version{Major: 1, Minor: 10, Patch: 3}},
	}
	for _, testCase := range cases {
		got, err := ParseVersion(testCase.text)
		if err != nil {
			t.Errorf("ParseVersion(%q) error = %v", testCase.text, err)
			continue
		}
		if got != testCase.want {
			t.Errorf("ParseVersion(%q) = %v, want %v", testCase.text, got, testCase.want)
		}
	}
}

func TestParseVersionRejectsInvalidText(t *testing.T) {
	for _, text := range []string{"", "julia", "1.x.3", "1.10.3.4", "-1.0.0", "1..3"} {
		if _, err := ParseVersion(text); err == nil {
			t.Errorf("ParseVersion(%q) error = nil, want an error", text)
		} else if !errors.Is(err, ErrInvalidVersion) {
			t.Errorf("ParseVersion(%q) error = %v, want it to wrap ErrInvalidVersion", text, err)
		}
	}
}

func TestVersionString(t *testing.T) {
	if got, want := (Version{Major: 1, Minor: 10, Patch: 3}).String(), "1.10.3"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := (Version{Major: 1, Minor: 11, Pre: "rc1"}).String(), "1.11.0-rc1"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		left  Version
		right Version
		want  int
	}{
		{left: Version{Major: 1, Minor: 10, Patch: 0}, right: Version{Major: 1, Minor: 9, Patch: 4}, want: 1},
		{left: Version{Major: 1, Minor: 9, Patch: 4}, right: Version{Major: 1, Minor: 10, Patch: 0}, want: -1},
		{left: Version{Major: 1, Minor: 10, Patch: 3}, right: Version{Major: 1, Minor: 10, Patch: 3}, want: 0},
		{left: Version{Major: 1, Minor: 11, Pre: "rc1"}, right: Version{Major: 1, Minor: 11}, want: -1},
		{left: Version{Major: 1, Minor: 11}, right: Version{Major: 1, Minor: 11, Pre: "rc1"}, want: 1},
	}
	for _, testCase := range cases {
		got := testCase.left.Compare(testCase.right)
		switch {
		case testCase.want < 0 && got >= 0:
			t.Errorf("%v.Compare(%v) = %d, want a negative number", testCase.left, testCase.right, got)
		case testCase.want > 0 && got <= 0:
			t.Errorf("%v.Compare(%v) = %d, want a positive number", testCase.left, testCase.right, got)
		case testCase.want == 0 && got != 0:
			t.Errorf("%v.Compare(%v) = %d, want 0", testCase.left, testCase.right, got)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	minimum := Version{Major: 1, Minor: 10}
	if !minimum.AtLeast(MinVersion) {
		t.Errorf("%v.AtLeast(%v) = false, want true", minimum, MinVersion)
	}
	older := Version{Major: 1, Minor: 9, Patch: 9}
	if older.AtLeast(MinVersion) {
		t.Errorf("%v.AtLeast(%v) = true, want false", older, MinVersion)
	}
}

func TestParseJuliaVersion(t *testing.T) {
	version, err := ParseJuliaVersion("julia version 1.10.3\n")
	if err != nil {
		t.Fatalf("ParseJuliaVersion() error = %v", err)
	}
	if got, want := version.String(), "1.10.3"; got != want {
		t.Errorf("ParseJuliaVersion() = %q, want %q", got, want)
	}
	if _, err := ParseJuliaVersion("the command failed"); err == nil {
		t.Error("ParseJuliaVersion() error = nil, want an error")
	}
}
