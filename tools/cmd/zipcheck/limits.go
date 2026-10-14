package main

// zipLimits are the sizes that the SDK's module zip must keep to. Raise one only when the growth
// is wanted, and give the reason next to it in the same change; CONTRIBUTING.md says how.
var zipLimits = limits{
	fileCap: 256 << 10,
	// Files that need more than the per-file cap, each with its own cap and the reason. None
	// does yet.
	exceptions: nil,
	budget: budget{
		baseline: 173560,
		measured: "2026-10-14",
		headroom: 256 << 10,
		reason: "The baseline is the zip of the commit that added zipcheck. The headroom leaves " +
			"room for the client and the first API methods without a raise, while growth of " +
			"more than 256 KiB fails, such as v1's 910 KB image split into files under the " +
			"per-file cap.",
	},
}
