package scan

import "strings"

// videoControllerIDs reads the adapter list from the Win32_VideoController
// query in displayAdapters: one PNPDeviceID per line. PowerShell prints a
// blank line for an adapter that reports no device ID, and the class also
// lists adapters that are not working (see ConfigManagerErrorCode).
func videoControllerIDs(out string) []string {
	var ids []string
	seen := make(map[string]bool, 4)
	for _, line := range strings.Split(out, "\n") {
		id := strings.TrimSpace(line)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// adapterKeys maps the adapters Windows reports to the display driver
// entries behind them, in the order Windows lists them. The "Driver" value
// of a device instance is the entry's key path, "{4d36e968-...}\0001".
// Cards that were taken out keep their entry, so the entries must never be
// read on their own: only those a present adapter points at count.
func adapterKeys(ids []string, driverValue func(id string) (string, bool)) []string {
	var keys []string
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		drv, ok := driverValue(id)
		if !ok {
			continue
		}
		key, ok := adapterKeyFromDriver(drv)
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

// adapterKeyFromDriver reads the entry number ("0001") from a device's
// "Driver" value. It reports false for a value that names no entry.
func adapterKeyFromDriver(drv string) (string, bool) {
	drv = strings.TrimSpace(drv)
	i := strings.LastIndex(drv, `\`)
	if i < 0 || i == len(drv)-1 {
		return "", false
	}
	return drv[i+1:], true
}
