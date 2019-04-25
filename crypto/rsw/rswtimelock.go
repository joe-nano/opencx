package rsw

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"math/big"

	"github.com/mit-dci/opencx/crypto"
)

// TimelockRSW generates the puzzle that can then only be solved with repeated squarings
type TimelockRSW struct {
	rsaKeyBits int
	key        []byte
	p          *big.Int
	q          *big.Int
	t          *big.Int
	a          *big.Int
}

// PuzzleRSW is the puzzle that can be then solved by repeated modular squaring
type PuzzleRSW struct {
	n *big.Int
	a *big.Int
	t *big.Int
	// We use C_k = b ⊕ k, I have add functionality but I don't know what's "better"
	ck *big.Int
}

// New creates a new TimelockRSW with p and q generated as per crypto/rsa, and an input a as well as number of bits for the RSA key size.
// The key is also set here
// The number of bits is so we can figure out how big we want p and q to be.
func New(key []byte, a int64, rsaKeyBits int) (timelock crypto.Timelock, err error) {
	tl := new(TimelockRSW)
	tl.rsaKeyBits = rsaKeyBits
	// generate primes p and q
	var rsaPrivKey *rsa.PrivateKey
	if rsaPrivKey, err = rsa.GenerateMultiPrimeKey(rand.Reader, 2, tl.rsaKeyBits); err != nil {
		err = fmt.Errorf("Could not generate primes for RSA: %s", err)
		return
	}
	if len(rsaPrivKey.Primes) != 2 {
		err = fmt.Errorf("For some reason the RSA Privkey has != 2 primes, this should not be the case for RSW, we only need p and q")
		return
	}
	tl.p = rsaPrivKey.Primes[0]
	tl.q = rsaPrivKey.Primes[1]
	tl.a = big.NewInt(a)
	tl.key = key

	timelock = tl
	return
}

// New2048 creates a new TimelockRSW with p and q generated as per crypto/rsa, and an input a. This generates according to a fixed RSA key size (2048 bits).
func New2048(key []byte, a int64) (tl crypto.Timelock, err error) {
	return New(key, a, 2048)
}

//New2048A2 is the same as New2048 but we use a base of 2. It's called A2 because A=2 I guess
func New2048A2(key []byte) (tl crypto.Timelock, err error) {
	return New(key, 2, 2048)
}

func (tl *TimelockRSW) n() (n *big.Int, err error) {
	if tl.p == nil || tl.q == nil {
		err = fmt.Errorf("Must set up p and q to get n")
		return
	}
	// n = pq
	n = new(big.Int).Mul(tl.p, tl.q)
	return
}

// ϕ() = phi(n) = (p-1)(q-1)
func (tl *TimelockRSW) ϕ() (ϕ *big.Int, err error) {
	if tl.p == nil || tl.q == nil {
		err = fmt.Errorf("Must set up p and q to get the ϕ")
		return
	}
	// ϕ(n) = (p-1)(q-1). We assume p and q are prime, and n = pq.
	ϕ = new(big.Int).Mul(new(big.Int).Sub(tl.p, big.NewInt(int64(1))), new(big.Int).Sub(tl.q, big.NewInt(1)))
	return
}

// e = 2^t (mod ϕ()) = 2^t (mod phi(n))
func (tl *TimelockRSW) e() (e *big.Int, err error) {
	if tl.t == nil {
		err = fmt.Errorf("Must set up t in order to get e")
		return
	}
	var ϕ *big.Int
	if ϕ, err = tl.ϕ(); err != nil {
		err = fmt.Errorf("Could not find ϕ: %s", err)
		return
	}
	// e = 2^t mod ϕ()
	e = new(big.Int).Exp(big.NewInt(int64(2)), tl.t, ϕ)
	return
}

// b = a^(e()) (mod n()) = a^e (mod n) = a^(2^t (mod ϕ())) (mod n) = a^(2^t) (mod n)
func (tl *TimelockRSW) b() (b *big.Int, err error) {
	if tl.a == nil {
		err = fmt.Errorf("Must set up a and n in order to get b")
		return
	}
	var n *big.Int
	if n, err = tl.n(); err != nil {
		err = fmt.Errorf("Could not find n: %s", err)
		return
	}
	var e *big.Int
	if e, err = tl.e(); err != nil {
		err = fmt.Errorf("Could not find e: %s", err)
		return
	}
	// b = a^(e()) (mod n())
	b = new(big.Int).Exp(tl.a, e, n)
	return
}

func (tl *TimelockRSW) ckXOR() (ck *big.Int, err error) {
	var b *big.Int
	if b, err = tl.b(); err != nil {
		err = fmt.Errorf("Could not find b: %s", err)
		return
	}
	// set k to be the bytes of the key
	k := new(big.Int).SetBytes(tl.key)

	// C_k = k ⊕ a^(2^t) (mod n) = k ⊕ b (mod n)
	ck = new(big.Int).Xor(b, k)
	return
}

func (tl *TimelockRSW) ckADD() (ck *big.Int, err error) {
	var b *big.Int
	if b, err = tl.b(); err != nil {
		err = fmt.Errorf("Could not find b: %s", err)
		return
	}
	// set k to be the bytes of the key
	k := new(big.Int).SetBytes(tl.key)

	// C_k = k + a^(2^t) (mod n) = k + b (mod n)
	// TODO: does this need to be ck.Add(b, k).Mod(ck, n)?
	ck = new(big.Int).Add(b, k)
	return
}

// SetupTimelockPuzzle sets up the time lock puzzle for the scheme described in RSW96. This uses the normal crypto/rsa way of selecting primes p and q.
// You should throw away the answer but some puzzles like the hash puzzle make sense to have the answer as an output of the setup, since that's the decryption key and you don't know beforehand how to encrypt.
func (tl *TimelockRSW) SetupTimelockPuzzle(t uint64) (puzzle crypto.Puzzle, answer []byte, err error) {
	tl.t = new(big.Int).SetUint64(t)
	var n *big.Int
	if n, err = tl.n(); err != nil {
		err = fmt.Errorf("Could not find n: %s", err)
		return
	}
	var ck *big.Int
	if ck, err = tl.ckXOR(); err != nil {
		err = fmt.Errorf("Could not find ck: %s", err)
		return
	}

	rswPuzzle := &PuzzleRSW{
		n:  n,
		a:  tl.a,
		t:  tl.t,
		ck: ck,
	}
	puzzle = rswPuzzle

	var b *big.Int
	if b, err = tl.b(); err != nil {
		err = fmt.Errorf("Could not find b: %s", err)
		return
	}
	// idk if this is a thing but ok
	answer = new(big.Int).Sub(ck, b).Bytes()
	return
}

// SolveCkADD solves the puzzle by repeated squarings and subtracting b from ck
func (pz *PuzzleRSW) SolveCkADD() (answer []byte, err error) {
	// One liner!
	return new(big.Int).Sub(pz.ck, new(big.Int).Exp(pz.a, new(big.Int).Exp(big.NewInt(2), pz.t, nil), pz.n)).Bytes(), nil
}

// SolveCkXOR solves the puzzle by repeated squarings and xor b with ck
func (pz *PuzzleRSW) SolveCkXOR() (answer []byte, err error) {
	// One liner!
	return new(big.Int).Xor(pz.ck, new(big.Int).Exp(pz.a, new(big.Int).Exp(big.NewInt(2), pz.t, nil), pz.n)).Bytes(), nil
}

// Solve solves the puzzle by repeated squarings
func (pz *PuzzleRSW) Solve() (answer []byte, err error) {
	return pz.SolveCkXOR()
}