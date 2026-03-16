//go:build !darwin

package filetree

func ReadClipboardFiles() []string {
	return nil
}
