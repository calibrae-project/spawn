package Duod

// This file is only to make "go get" working

import (
	// comment
	_ "github.com/dchest/siphash"
	_ "github.com/golang/snappy"
	_ "golang.org/x/crypto/ripemd160"
	_ "github.com/fatih/color"
)

// Version -
const Version = "1.9.5"
