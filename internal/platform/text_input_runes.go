package platform

import "unicode/utf16"

// RunesInUTF16 returns how many runes of s the first units UTF-16 units hold.
func RunesInUTF16(s string, units int) int {
	n := 0
	for _, r := range s {
		if units <= 0 {
			break
		}
		units -= max(utf16.RuneLen(r), 1)
		n++
	}
	return n
}
