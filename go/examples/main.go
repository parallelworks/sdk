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

	parallelworks "github.com/parallelworks/client-go"
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
	bucketsResp, err := client.GetBucketsWithResponse(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to get buckets: %v", err)
	}

	if bucketsResp.StatusCode() != 200 {
		log.Fatalf("Failed to get buckets: status %d", bucketsResp.StatusCode())
	}

	buckets := bucketsResp.JSON200
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
	clustersResp, err := client.GetClustersWithResponse(ctx)
	if err != nil {
		log.Fatalf("Failed to get clusters: %v", err)
	}

	if clustersResp.StatusCode() != 200 {
		log.Fatalf("Failed to get clusters: status %d", clustersResp.StatusCode())
	}

	clusters := clustersResp.JSON200
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
