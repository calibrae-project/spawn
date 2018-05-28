package main

/*
  This is a ECVerify speedup that is advised for non Windows systems.

  1) Build and install sipa's secp256k1 lib for your system

  2) Copy this file one level up and remove "speedup.go" from there

  3) Rebuild clinet.exe and enjoy sipa's verify lib.
*/

import (
	"github.com/ParallelCoinTeam/duod/lib/btc"
	"github.com/ParallelCoinTeam/duod/lib/logg"
	"github.com/ParallelCoinTeam/duod/lib/others/cgo/sipasec"
)

// ECVerify -
func ECVerify(k, s, h []byte) bool {
	return sipasec.ECVerify(k, s, h) == 1
}

func init() {
	logg.Debug.Println("Using libsecp256k1.a by sipa for ECVerify")
	btc.ECVerify = ECVerify
}
