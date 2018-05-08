package btc

const (
	// Coin - Precision of one token
	Coin = 1e8
	// MaxTokenSupply -
	MaxTokenSupply = 21000000 * Coin
	// MaxBlockWeight -
	MaxBlockWeight = 4e6
	// MessageMagic -
	MessageMagic = "Bitcoin Signed Message:\n"
	// LockTimeThreshold -
	LockTimeThreshold = 500000000
	// MaxScriptElementSize -
	MaxScriptElementSize = 520
	// MaxBlockSigOpsCost -
	MaxBlockSigOpsCost = 80000
	// MaxPubKeysPerMultisig -
	MaxPubKeysPerMultisig = 20
	// WitnessScaleFactor -
	WitnessScaleFactor = 4
)
