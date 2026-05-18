//go:build recording && mockkc

package main

import "github.com/zalando/go-keyring"

func init() {
	keyring.MockInit()
}
