// Example: List organizations using the Parallel Works Go client
//
// Usage:
//
//	export PW_API_KEY="your-api-key"
//	export PW_HOST="https://cloud.parallel.works"  # optional, defaults to cloud.parallel.works
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	parallelworks "github.com/parallelworks/client-go"
)

func main() {
	apiKey := os.Getenv("PW_API_KEY")
	if apiKey == "" {
		log.Fatal("PW_API_KEY environment variable is required")
	}

	// Create an authenticated client
	client, err := parallelworks.NewClientFromCredential(
		apiKey,
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// List organizations
	fmt.Println("Fetching organizations...")
	resp, err := client.GetOrganizationsWithResponse(context.Background())
	if err != nil {
		log.Fatalf("Failed to get organizations: %v", err)
	}

	if resp.StatusCode() != 200 {
		log.Fatalf("Unexpected status code: %d", resp.StatusCode())
	}

	if resp.JSON200 == nil {
		fmt.Println("No organizations found")
		return
	}

	fmt.Printf("\nFound %d organization(s):\n\n", len(*resp.JSON200))
	for _, org := range *resp.JSON200 {
		fmt.Printf("  - %s (ID: %s)\n", org.Name, org.Id)
	}
}
