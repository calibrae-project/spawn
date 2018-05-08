package chain

const (
	// BlockMapInitLen -
	BlockMapInitLen = 500e3
	// MovingCheckopintDepth -
	MovingCheckopintDepth = 2016 // Do not accept forks that wold go deeper in a past
	// BIP16SwitchTime -
	BIP16SwitchTime = 1333238400 // BIP16 didn't become active until Apr 1 2012
	// CoinbaseMaturity -
	CoinbaseMaturity = 23
	// MedianTimeSpan -
	MedianTimeSpan = 11
)
