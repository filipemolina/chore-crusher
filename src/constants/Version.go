package constants

import "runtime/debug"

// version is stamped at build time:
//
//	go build -ldflags "-X github.com/filipemolina/chore-completer/src/constants.version=v0.1.0"
//
// which is what the Makefile and the release build do. It is unexported so
// the stamp has exactly one reader, Version(), and every caller gets the same
// answer whether or not the stamp happened. Ported verbatim from
// stack-stitcher's src/constants/Version.go — see docs/plans/phase-0-scaffolding.md
// step 4.
var version string

// Version reports the running build's version.
//
// A stamped value wins - that is a release, and it says so. Otherwise the
// build info answers, and which half of it answers sorts the two remaining
// cases cleanly:
//
//   - Built from a checkout: there is a vcs.revision, so the short commit is
//     the version. The toolchain also synthesizes a v0.0.0-<date>-<hash>
//     pseudo-version here, which says the same thing at three times the
//     length and reads like a release that never existed.
//   - Installed with `go install ...@v0.1.0`: a module download has no VCS
//     info at all, so Main.Version is the only answer, and it is the right
//     one.
func Version() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	var revision, modified string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}

	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		if modified == "true" {
			// Uncommitted changes: the commit alone would name a build that
			// does not exist anywhere.
			return revision + "-dirty"
		}

		return revision
	}

	// (devel) is what the toolchain writes when it has nothing better, and it
	// is nothing better.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	return "unknown"
}
