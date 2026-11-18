package privilege

import "fmt"

const (
	// systemIDHigh and systemIDLow bound the safe UID/GID range on Linux and BSD
	systemIDHigh = 899
	systemIDLow  = 200
	// macOSIDHigh and macOSIDLow bound the safe UID/GID range on macOS
	macOSIDHigh = 399
	macOSIDLow  = 200
)

// reservedIDs are UIDs and GIDs used by well-known services across distributions.
// They are never chosen even when they look free on this system (AI.md PART 24
// "Reserved/Well-Known UIDs").
var reservedIDs = map[int]bool{
	65534: true,
	999:   true, 998: true, 997: true, 996: true, 995: true,
	994: true, 993: true, 992: true, 991: true, 990: true,
	989: true, 988: true, 987: true, 986: true, 985: true,
	984: true, 983: true, 982: true, 981: true, 980: true,
	101: true, 102: true, 103: true, 104: true, 105: true,
	106: true, 107: true, 108: true, 109: true, 110: true,
	170: true, 171: true, 172: true, 173: true, 174: true,
	175: true, 176: true, 177: true, 178: true, 179: true,
}

// IsReservedID reports whether an ID belongs to a well-known service.
func IsReservedID(id int) bool {
	return reservedIDs[id]
}

// FindAvailableSystemID scans from high down to low and returns the first ID that is
// not reserved and is free as both a UID and a GID (AI.md PART 24 "UID/GID Selection Logic").
func FindAvailableSystemID(high, low int, isUIDTaken, isGIDTaken func(int) bool) (int, error) {
	for id := high; id >= low; id-- {
		if reservedIDs[id] {
			continue
		}
		if isUIDTaken(id) || isGIDTaken(id) {
			continue
		}
		return id, nil
	}
	return 0, fmt.Errorf("no available UID/GID in safe range %d-%d", low, high)
}
