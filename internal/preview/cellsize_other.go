//go:build !unix

package preview

// detectCellSize has no portable ioctl on non-unix platforms, so it uses the
// default cell size. Graphics previews still work, just sized from the default.
func detectCellSize() (int, int) {
	return defaultCellW, defaultCellH
}
