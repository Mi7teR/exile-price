//go:build !windows

package main

import (
	"fmt"
	"os"
)

// The overlay runs only on Windows; elsewhere the package builds so that
// its platform-free parts can be tested.
func main() {
	fmt.Fprintln(os.Stderr, "exile-price: the overlay runs on Windows only; see cmd/pricecheck-cli")
	os.Exit(1)
}
