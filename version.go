package tailkit

import "strings"

// versionAtLeast reports whether have >= want using a simple semver comparison.
// Handles "v" prefix and zero-pads components so "1.10.0" > "1.9.0" correctly.
func versionAtLeast(have, want string) bool {
	have = strings.TrimPrefix(have, "v")
	want = strings.TrimPrefix(want, "v")
	hp := strings.Split(have, ".")
	wp := strings.Split(want, ".")
	for len(hp) < len(wp) {
		hp = append(hp, "0")
	}
	for len(wp) < len(hp) {
		wp = append(wp, "0")
	}
	for i := range hp {
		h := zeroPad(hp[i], len(wp[i]))
		w := zeroPad(wp[i], len(hp[i]))
		if h > w {
			return true
		}
		if h < w {
			return false
		}
	}
	return true
}

func zeroPad(s string, width int) string {
	for len(s) < width {
		s = "0" + s
	}
	return s
}
