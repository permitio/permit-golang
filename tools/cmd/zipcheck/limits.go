package main

// zipLimits are the sizes that the SDK's module zip must keep to. Raise one only when the growth
// is wanted, and give the reason next to it in the same change; CONTRIBUTING.md says how.
var zipLimits = limits{
	fileCap: 256 << 10,
	// Files that need more than the per-file cap, each with its own cap and the reason.
	exceptions: []exception{
		{
			path:     "MIGRATION.md",
			maxBytes: 320 << 10,
			reason: "The migration guide has an entry for each identifier, operation and models " +
				"type of v1.3.0, 822 in all, and an appendix that maps each models type, so " +
				"that code moving from v1 finds every name it uses. It was 283961 bytes when it " +
				"was finished, and it changes after v2.0.0 only to correct an entry.",
		},
	},
	budget: budget{
		baseline: 1231982,
		measured: "2026-10-16",
		headroom: 256 << 10,
		reason: "The first baseline, 173560 bytes, was the zip of the commit that added " +
			"zipcheck; the client core (errors, options, logging, transport, retries, models " +
			"and pages, with their tests) used its headroom, and the second, 436043 bytes, " +
			"was the zip of the commit that added list pages. The decisions, the end-to-end " +
			"suite and the first API services (projects, environments, API keys, resources), " +
			"with their tests, docs and migration entries, used that headroom, and the third, " +
			"727653 bytes, was the zip of the commit that added the resources API. The schema " +
			"services (resource actions, action groups, resource and user attributes, " +
			"relations, roles) used most of the next headroom, and the fourth, 914366 bytes, " +
			"was the zip of the commit that added role bulk replace. The condition sets, " +
			"resource roles, implicit grants, users and proxy configs services used most of " +
			"that headroom, and the fifth, 1096575 bytes, was the zip of the commit that added " +
			"user role assignment. Tenant membership, role assignments, resource instances, " +
			"relationship tuples and the bulk operations used half of the next headroom. The " +
			"baseline is now the zip of the commit that added bulk relationship tuples, and the " +
			"headroom leaves room for the next API services, while growth of more than 256 KiB " +
			"fails, such as v1's 910 KB image split into files under the per-file cap.",
	},
}
