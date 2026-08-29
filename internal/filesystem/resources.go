package filesystem

import "os"

// HasEntries reports whether a directory holds anything at all. It is what the
// board's resource mark asks: a folder that exists but is empty promises
// something and delivers nothing, so it is not worth marking.
//
// Readdirnames(1) stops at the first entry, so a folder with three thousand
// files costs the same as one with one -- which matters, because this runs for
// every visible card on every render.
func HasEntries(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()

	names, err := f.Readdirnames(1)
	return err == nil && len(names) > 0
}
