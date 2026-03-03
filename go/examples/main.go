// Example: List buckets and clusters using the Parallel Works Go client
//
// Usage:
//
//	export PW_API_KEY="your-api-key-or-token"
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	parallelworks "github.com/parallelworks/sdk/go/v7"
)

func main() {
	apiKey := os.Getenv("PW_API_KEY")
	if apiKey == "" {
		log.Fatal("PW_API_KEY environment variable is required")
	}

	// Create an authenticated client - host is auto-detected from credential
	client, err := parallelworks.NewClientFromCredential(apiKey)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	fmt.Println("Fetching resources...")
	fmt.Println()

	// List buckets
	buckets, err := client.GetBuckets(ctx)
	if err != nil {
		log.Fatalf("Failed to get buckets: %v", err)
	}

	if buckets == nil || len(*buckets) == 0 {
		fmt.Println("Buckets (0):")
		fmt.Println("  No buckets found")
	} else {
		fmt.Printf("Buckets (%d):\n", len(*buckets))
		for _, bucket := range *buckets {
			fmt.Printf("  - %s (%s)\n", bucket.Name, bucket.Csp)
		}
	}

	fmt.Println()

	// List clusters
	clusters, err := client.GetClusters(ctx)
	if err != nil {
		log.Fatalf("Failed to get clusters: %v", err)
	}

	if clusters == nil || len(*clusters) == 0 {
		fmt.Println("Clusters (0):")
		fmt.Println("  No clusters found")
	} else {
		fmt.Printf("Clusters (%d):\n", len(*clusters))
		for _, cluster := range *clusters {
			fmt.Printf("  - %s (%s)\n", cluster.Name, cluster.Status)
		}
	}
}
