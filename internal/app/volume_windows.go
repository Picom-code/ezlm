package app

import "golang.org/x/sys/windows"

// volumeLabel is the drive's name in Explorer ("Data", "Games"), if it has one.
func volumeLabel(root string) string {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 261)
	if windows.GetVolumeInformation(p, &buf[0], uint32(len(buf)), nil, nil, nil, nil, 0) != nil {
		return ""
	}
	return windows.UTF16ToString(buf)
}
