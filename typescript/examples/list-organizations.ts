/// <reference types="node" />
/**
 * Example: List organizations using the Parallel Works TypeScript client
 *
 * Usage:
 *   export PW_API_KEY="your-api-key-or-token"
 *   pnpm tsx list-organizations.ts
 */

import { Client } from "@parallelworks/client";

async function main() {
	const apiKey = process.env.PW_API_KEY;
	if (!apiKey) {
		console.error("Error: PW_API_KEY environment variable is required");
		process.exit(1);
	}

	// Create an authenticated client - host is auto-detected from credential
	const client = Client.fromCredential(apiKey);

	console.log("Fetching organizations...");

	const { data, error } = await client.GET("/api/organizations");

	if (error) {
		console.error("Failed to get organizations:", error);
		process.exit(1);
	}

	if (!data || data.length === 0) {
		console.log("No organizations found");
		return;
	}

	console.log(`\nFound ${data.length} organization(s):\n`);
	for (const org of data) {
		console.log(`  - ${org.name} (ID: ${org.id})`);
	}
}

main();
