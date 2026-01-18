#!/usr/bin/env python3
"""
Example: List buckets and clusters using the Parallel Works Python client

Usage:
    export PW_API_KEY="your-api-key-or-token"
    uv run list_resources.py
"""

import os
import sys

from parallelworks_client import Client


def main():
    api_key = os.environ.get("PW_API_KEY")
    if not api_key:
        print("Error: PW_API_KEY environment variable is required", file=sys.stderr)
        sys.exit(1)

    # Create an authenticated client - host is auto-detected from credential
    with Client.from_credential(api_key).sync() as client:
        print("Fetching resources...\n")

        # Fetch buckets
        buckets_response = client.get("/api/buckets")
        buckets_response.raise_for_status()
        buckets = buckets_response.json()

        print(f"Buckets ({len(buckets)}):")
        if not buckets:
            print("  No buckets found")
        else:
            for bucket in buckets:
                print(f"  - {bucket['name']} ({bucket['csp']})")

        print()

        # Fetch clusters
        clusters_response = client.get("/api/clusters")
        clusters_response.raise_for_status()
        clusters = clusters_response.json()

        print(f"Clusters ({len(clusters)}):")
        if not clusters:
            print("  No clusters found")
        else:
            for cluster in clusters:
                print(f"  - {cluster['name']} ({cluster['status']})")


if __name__ == "__main__":
    main()
