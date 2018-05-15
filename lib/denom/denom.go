// Package denom is a library for processing, storing and computing 256 bit big integers for the denomination of the Spawn token, based on the code from math/big/int.go. The denomination is fixed precision, with 8 bytes for the whole number part and 24 bytes for the decimals, and allows for sufficient precision for around 30 years of 3.125% (1/32) annual supply expansion  and block reward reduction with exponential decay.
package denom

// Word is a generic binary type that can store any integer
type Word uintptr

const(
	// Compute the size _S of a Word in bytes.
	_m    = ^Word(0)
	_logS = _m>>8&1 + _m>>16&1 + _m>>32&1
	_S    = 1 << _logS

	_W = _S << 3 // word size in bits
	_B = 1 << _W // digit base
	_M = _B - 1  // digit mask

	_W2 = _W / 2   // half word size in bits
	_B2 = 1 << _W2 // half digit base
	_M2 = _B2 - 1  // half digit mask
)

// Scalar is an arbitrary sized integer
type Scalar []Word

var (
	// Zero is an empty scalar
	Zero 	= Int{false, 	nil}
	// One is the identity scalar
	One 	= Int{false, 	Scalar{1}}
	// Two is for binary operations
	Two 	= Int{false, 	Scalar{2}}
	// Ten is for decimal operations
	Ten 	= Int{false, 	Scalar{10}}
)

// Int is a scalar with direction (sign)
type Int struct {
	neg bool
	scalar Scalar
}

// Sign of an Int - returns 1, 0 or -1
func (I *Int) Sign() int {
	switch{
	case len(I.scalar) == 0:
		return 0
	case I.neg:
		return -1
	default:
		return 1
	}
}

// Make a new scalar with a specified length and return it
func (S Scalar) Make(length int) Scalar{
	if length<= cap(S) { S = S[0:length] }
	const e=4
	return make(Scalar, length, length+e)
}

// Assign an Scalar to a Scalar
func (S Scalar) Assign(s Scalar) {
	S = S.Make(len(s))
	copy(S,s)
}

// Assign an Int to an Int and return it
func (I *Int) Assign(i *Int) *Int {
	if I != i {
		I.scalar.Assign(i.scalar)
		I.neg = i.neg
	}
	return I
}

// AssignWord puts a Word into a Scalar and returns it
func (S Scalar) AssignWord(w Word) {
	if w == 0 { 
		S = S.Make(0) 
	} else {
		S = S.Make(1)
		S[0] = w
	}
}

// AssignUint64 puts a uint64 into a Scalar
func (S Scalar) AssignUint64(u uint64) Scalar {
	if uint64(Word(u)) != u {	
		n := 0
		for t := u; t > 0; t>>=_W { n++ }

		S = S.Make(n)
		for i := range S {
			S[i] = Word(u&_M)
			u >>=_W
		}
	}
	return S
}

// AssignInt64 puts an int64 into an Int and returns the Int
func (I *Int) AssignInt64(i int64) *Int {
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	I.scalar.AssignUint64(uint64(i))
	I.neg = neg
	return I
}

// NewInt allocates and returns a new Int set to an int64 value
func NewInt(i int64) *Int {
	return new(Int).AssignInt64(i)
}

// Abs takes an int and returns it only as positive
func (I *Int) Abs() *Int {
	I.neg = false
	return I
}

// Neg flips the sign if Int is nonzero and returns it
func (I *Int) Neg() *Int {
	I.neg = len(I.scalar) > 0 && !I.neg
	return I
}

// Compare a scalar to another, returns 1 if parameter is greater, 0 if equal, -1 if lesser
func (S Scalar) Compare(s Scalar) (r int) {
	L, l := len(S), len(s)
	if L != l || L == 0 {
		switch{
		case L < l:
			r = -1
		case L > l:
			r = 1
		}
		return
	}
	i := L - 1
	for i > 0 && S[i] == s[i] { i-- }
	switch{
	case S[i] < s[i]:
		r = -1
	case S[i] > s[i]:
		r = 1
	}
	return
}

// Norm normalises a scalar
func (S Scalar) Norm() Scalar {
	i := len(S)
	for i>0 && S[i-1]==0 { i-- }
	return S[0:i]
}

// Add a scalar to this scalar and return the result
func (S Scalar) Add(s Scalar) Scalar {
	L, l := len(S), len(s)
	switch {
	case L < l:
		return s.Add(S)
	case L == 0:
		return S.Make(0)
	case l == 0:
		return S
	}
	z := S.Make(L + 1)
	c := addVV(z[0:l], S, s)
	if L > l { addVW(z[l:L], z[l:], c) }
	z[L] = c
	return S.Norm()
}

// Sub tract a scalar from this scalar and return the result
func (S Scalar) Sub(s Scalar) Scalar {
	L, l := len(S), len(s)
	switch{
	case L<l:
		panic("underflow")
	case L==0:
		return S.Make(0)
	case l==0:
		return S
	}
	z := S.Make(L)
	c := subVV(z[0:l], S, s)
	if L > l { c = subVW(z[l:], S, c) }
	if c != 0 { panic("underflow") }
	return z.Norm()
}

// Add adds another Int to this one and returns the result
func (I *Int) Add(i *Int) *Int {
	switch{
	case len(i.scalar) == 0 && len(I.scalar) > 0:
		return I
	case len(i.scalar) > 0 && len(I.scalar) == 0:
		return I.Assign(i)
	case I.neg == i.neg:
		r := new(Int)
		r.scalar = I.scalar.Add(i.scalar)
		r.neg = I.neg
		return r
	case I.scalar.Compare(i.scalar) >= 0:
		r := new(Int)
		r.scalar = I.scalar.Sub(i.scalar)
		r.neg = I.neg
		return r
	}
	r := new(Int)
	r.scalar = i.scalar.Sub(I.scalar)
	r.neg = !I.neg
	return r
}