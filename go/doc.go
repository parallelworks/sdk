// Package parallelworks provides a Go client for the Parallel Works ACTIVATE platform API.
//
// Create a client using NewClientFromCredential for automatic host detection:
//
//	client, err := parallelworks.NewClientFromCredential(os.Getenv("PW_API_KEY"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Or use NewClient for explicit configuration:
//
//	client := parallelworks.NewClient(
//	    "https://cloud.parallel.works",
//	    parallelworks.WithAuth(&parallelworks.BasicAuth{Username: "pwt_..."}),
//	)
//
// For full API documentation, visit https://parallelworks.com/docs.
package parallelworks
