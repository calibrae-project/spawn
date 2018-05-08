package chain

const (
	// DebugWasted -
	DebugWasted = 1 << 0
	// DebugUnspent -
	DebugUnspent = 1 << 1
	// DebugBlocks -
	DebugBlocks = 1 << 2
	// DebugOrphans -
	DebugOrphans = 1 << 3
	// DebugTx -
	DebugTx = 1 << 4
	// DebugScript -
	DebugScript = 1 << 5
	// DebugVerify -
	DebugVerify = 1 << 6
	// DebugScrErr -
	DebugScrErr = 1 << 7
)

var dbgmask uint32

func don(b uint32) bool {
	return (dbgmask & b) != 0
}

// DbgSwitch -
func DbgSwitch(b uint32, on bool) {
	if on {
		dbgmask |= b
	}
	dbgmask ^= (b & dbgmask)
}
