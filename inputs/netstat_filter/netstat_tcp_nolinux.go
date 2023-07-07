//go:build !linux
// +build !linux

package netstat

func Parse(proto string) ([]Entry, error) {
	entries := make([]Entry, 0)

	entries = append(entries, NewEntry(
		proto,
		nil,
		0,
		nil,
		0,
		0,
		0,
		0,
		0,
	))

	return entries, nil
}
