package chain

const (
	DebugWasted  = 1 << 0
	DebugUnspent = 1 << 1
	DebugBlocks  = 1 << 2
	DebugOrphans = 1 << 3
	DebugTx      = 1 << 4
	DebugScript  = 1 << 5
	DebugVerify  = 1 << 6
	DebugScrErr  = 1 << 7
)

var dbgmask uint32 = 0

func don(b uint32) bool {
	return (dbgmask & b) != 0
}

func DbgSwitch(b uint32, on bool) {
	if on {
		dbgmask |= b
	} else {
		dbgmask ^= (b & dbgmask)
	}
}
