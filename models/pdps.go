package models

// PDPDataRefreshRequest is a refresh of the data of the PDPs of an environment, as
// PDPsService.Refresh and PDPsService.RefreshPDP of the root package send it. Each field that
// is its zero value (empty for Reason, nil for ShardID) is left out of the request, so the zero
// PDPDataRefreshRequest sends {}.
type PDPDataRefreshRequest struct {
	// Reason says why the data is refreshed, at most 512 characters. Empty leaves it out.
	Reason string `json:"reason,omitzero"`

	// ShardID is the shard of the PDP whose data to refresh, 0 or more: a pointer to 0 sends 0,
	// and nil leaves it out. Only a refresh of one PDP takes it: the API's documentation says
	// that a refresh of every PDP of the environment does not, and the API rejects it there.
	ShardID *int `json:"shard_id,omitzero"`
}

// PDPDataRefreshResponse is the reply to a refresh of the data of the PDPs of an environment:
// the API has asked the PDPs to read their data again, and the reply does not wait until they
// have.
type PDPDataRefreshResponse struct {
	// UpdateID is the ID of the update that asks the PDPs to read their data again, a UUID.
	UpdateID string `json:"update_id"`

	// PDPIDs are the IDs of the PDPs that the update asks, each a UUID.
	PDPIDs []string `json:"pdp_ids"`
}
