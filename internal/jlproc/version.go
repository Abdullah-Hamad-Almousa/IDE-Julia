package jlproc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidVersion reports that a string does not hold a Julia version.
var ErrInvalidVersion = errors.New("jlproc: the string is not a Julia version")

// Version is a Julia release version.
//
// A Julia version has the form "major.minor.patch" with an optional
// pre-release suffix, for example "1.11.0-rc1". A pre-release is older than
// the release it belongs to, as the semantic versioning rules require.
type Version struct {
	// Major is the major component of the version.
	Major int
	// Minor is the minor component of the version.
	Minor int
	// Patch is the patch component of the version.
	Patch int
	// Pre is the pre-release suffix without the leading dash. It is empty
	// for a final release.
	Pre string
}

// MinVersion is the oldest Julia release that jide supports.
//
// Decision D5 of the plan pins the minimum to the 1.10 long term support
// release.
var MinVersion = Version{Major: 1, Minor: 10, Patch: 0}

// ParseVersion parses the version part of a Julia version string.
//
// The parser accepts a bare version such as "1.10.3", a version with a
// pre-release suffix such as "1.11.0-rc1" and a version with build metadata
// such as "1.10.3+build". It rejects everything else.
func ParseVersion(text string) (Version, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Version{}, fmt.Errorf("jlproc: %w: the version is empty", ErrInvalidVersion)
	}
	if index := strings.IndexByte(trimmed, '+'); index >= 0 {
		trimmed = trimmed[:index]
	}
	pre := ""
	if index := strings.IndexByte(trimmed, '-'); index >= 0 {
		pre = trimmed[index+1:]
		trimmed = trimmed[:index]
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return Version{}, fmt.Errorf("jlproc: %w: %q", ErrInvalidVersion, text)
	}
	numbers := [3]int{}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return Version{}, fmt.Errorf("jlproc: %w: %q", ErrInvalidVersion, text)
		}
		numbers[index] = number
	}
	return Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2], Pre: pre}, nil
}

// String returns the version in the "major.minor.patch" form, with the
// pre-release suffix when the version has one.
func (v Version) String() string {
	base := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Pre != "" {
		return base + "-" + v.Pre
	}
	return base
}

// Compare compares two versions.
//
// It returns a negative number when the receiver is older than other, zero
// when both versions are equal and a positive number when the receiver is
// newer than other. Two different pre-release suffixes of the same version
// are ordered by their text.
func (v Version) Compare(other Version) int {
	switch {
	case v.Major != other.Major:
		return compareInt(v.Major, other.Major)
	case v.Minor != other.Minor:
		return compareInt(v.Minor, other.Minor)
	case v.Patch != other.Patch:
		return compareInt(v.Patch, other.Patch)
	}
	switch {
	case v.Pre == other.Pre:
		return 0
	case v.Pre == "":
		return 1
	case other.Pre == "":
		return -1
	default:
		return strings.Compare(v.Pre, other.Pre)
	}
}

// AtLeast reports whether the version is at least as new as other.
func (v Version) AtLeast(other Version) bool {
	return v.Compare(other) >= 0
}

// compareInt compares two integers without an import of the sort package.
func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
