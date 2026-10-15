package models

// DerivedRoleRuleDelete is a rule that grants a role to the users that have a role on a related
// resource, as ImplicitGrantsService.Delete of the root package sends it to delete the rule.
// Role, OnResource and LinkedByRelation are required, and the request always holds them.
type DerivedRoleRuleDelete struct {
	// Role is the key of the role that a user has on the related resource.
	Role string `json:"role"`

	// OnResource is the key of the related resource.
	OnResource string `json:"on_resource"`

	// LinkedByRelation is the key of the relation to the related resource.
	LinkedByRelation string `json:"linked_by_relation"`

	// When are the settings of the rule, which the API's documentation lists in the request of a
	// delete too. Nil leaves them out.
	When *DerivationSettings `json:"when,omitzero"`
}
