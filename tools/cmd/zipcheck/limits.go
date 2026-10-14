package main

// zipLimits are the sizes that the SDK's module zip must keep to. Raise one only when the growth
// is wanted, and give the reason next to it in the same change; CONTRIBUTING.md says how.
var zipLimits = limits{
	fileCap: 256 << 10,
	// Files that need more than the per-file cap, each with its own cap and the reason. None
	// does yet.
	exceptions: nil,
	budget: budget{
		baseline: 436043,
		measured: "2026-10-14",
		headroom: 256 << 10,
		reason: "The first baseline, 173560 bytes, was the zip of the commit that added " +
			"zipcheck; the client core (errors, options, logging, transport, retries, models " +
			"and pages, with their tests) used its headroom. The baseline is now the zip of " +
			"the commit that added list pages, and the headroom leaves room for the API " +
			"methods, while growth of more than 256 KiB fails, such as v1's 910 KB image " +
			"split into files under the per-file cap.",
	},
}
