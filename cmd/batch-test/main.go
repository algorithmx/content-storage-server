package main

import (
	"fmt"
	"os"

	"content-storage-server/cmd/batch-test/internal" // Corrected import path
)

func main() {
	fmt.Println("Running all batch tests...")

	// Run the simple batch fix test
	fmt.Println("\n--- Running Simple Batch Fix Test ---")
	internal.RunSimpleBatchFixTest()

	// Run the batch race fix test
	fmt.Println("\n--- Running Batch Race Fix Test ---")
	internal.RunBatchRaceFixTest()

	fmt.Println("\nAll batch tests completed.")
	os.Exit(0)
}
