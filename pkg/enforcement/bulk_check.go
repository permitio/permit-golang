package enforcement

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type opaBulkResponse struct {
	Result *allowBulkResponse `json:"result"`
}

type allowBulkResponse struct {
	Allow *[]CheckResponse `json:"allow"`
}

func NewBulkCheckRequest(requests ...CheckRequest) []CheckRequest {
	return requests
}

func NewBulkCheckRequestParameterized(user []User, action []Action, resource []Resource, context []map[string]string) ([]CheckRequest, error) {
	if len(user) != len(action) || len(user) != len(resource) || len(user) != len(context) {
		return nil, fmt.Errorf("user, action, resource and context must have the same length")
	}
	requests := make([]CheckRequest, len(user))
	for i := range user {
		requests[i] = CheckRequest{
			User:     user[i],
			Action:   action[i],
			Resource: resource[i],
			Context:  context[i],
		}
	}
	return requests, nil
}

func newJsonBulkCheckRequest(opaUrl string, requests ...CheckRequest) ([]byte, error) {
	checkReq := NewBulkCheckRequest(requests...)
	var genericCheckReq interface{} = checkReq
	if opaUrl != "" {
		genericCheckReq = &struct {
			Input []CheckRequest `json:"input"`
		}{checkReq}
	}
	jsonCheckReq, err := json.Marshal(genericCheckReq)
	if err != nil {
		return nil, err
	}
	return jsonCheckReq, nil
}

func (e *PermitEnforcer) getBulkCheckEndpoint() string {
	return e.getEndpointByPolicyPackage(bulkPolicyPackage)
}

func (e *PermitEnforcer) parseBulkResponse(res *http.Response) ([]CheckResponse, error) {
	var result []CheckResponse
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, res)
		e.logger.Error("error reading Permit.BulkCheck() response from PDP", zap.Error(permitError))
		return nil, permitError
	}
	err = errors.HttpErrorHandle(err, res)
	if err != nil {
		e.logger.Error(string(bodyBytes), zap.Error(err))
		return nil, err
	}
	if e.config.GetOpaUrl() != "" {
		opaStruct := &opaBulkResponse{
			&allowBulkResponse{
				&result,
			},
		}

		if err := json.Unmarshal(bodyBytes, opaStruct); err != nil {
			permitError := errors.NewPermitUnexpectedError(err, res)
			e.logger.Error("error unmarshalling Permit.BulkCheck() response from OPA", zap.Error(permitError))
			return nil, err
		}
	} else {
		pdpStruct := &allowBulkResponse{&result}
		if err := json.Unmarshal(bodyBytes, &pdpStruct); err != nil {
			permitError := errors.NewPermitUnexpectedError(err, res)
			e.logger.Error("error unmarshalling Permit.BulkCheck response from PDP", zap.Error(permitError))
			return nil, permitError
		}
	}

	return result, nil
}

func (e *PermitEnforcer) BulkCheck(requests ...CheckRequest) ([]bool, error) {
	tenantRequestsMap := make(map[string][]CheckRequest)
	requestsOrder := make(map[string]map[int]int)

	for i, request := range requests {
		// Create a map of tenant key to requests for that tenant
		tenantRequests := tenantRequestsMap[request.Resource.GetTenant()]
		if tenantRequests == nil {
			tenantRequests = []CheckRequest{
				request,
			}
		} else {
			tenantRequests = append(tenantRequests, request)
		}
		tenantRequestsMap[request.Resource.GetTenant()] = tenantRequests
		// keep a mapping between the index in the tenant slice to the original requests slice
		// so we can return the results in the same order as the original requests
		if requestsOrder[request.Resource.GetTenant()] == nil {
			requestsOrder[request.Resource.GetTenant()] = map[int]int{
				len(tenantRequests) - 1: i,
			}
		} else {
			requestsOrder[request.Resource.GetTenant()][len(tenantRequests)-1] = i
		}
	}
	results := make([]bool, len(requests))
	for tenant, tenantRequests := range tenantRequestsMap {
		tenantResults, err := e.bulkCheck(tenantRequests...)
		if err != nil {
			return nil, err
		}
		for i := range tenantRequests {
			// i represents the index in the tenant slice
			// put the result in the original index in the results slice
			// so we can return the results in the same order as the original requests
			originalRequestIndex := requestsOrder[tenant][i]
			results[originalRequestIndex] = tenantResults[i]
		}
	}
	return results, nil
}

func (e *PermitEnforcer) bulkCheck(requests ...CheckRequest) ([]bool, error) {
	reqAuthValue := "Bearer " + e.config.GetToken()

	jsonCheckReq, err := newJsonBulkCheckRequest(e.config.GetOpaUrl(), requests...)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error marshalling Permit.BulkCheck() request", zap.Error(permitError))
		return nil, permitError
	}
	reqBody := bytes.NewBuffer(jsonCheckReq)
	httpRequest, err := http.NewRequest(reqMethod, e.getBulkCheckEndpoint(), reqBody)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error creating Permit.BulkCheck() request", zap.Error(permitError))
		return nil, permitError
	}
	httpRequest.Header.Set(reqContentTypeKey, reqContentTypeValue)
	httpRequest.Header.Set(reqAuthKey, reqAuthValue)
	res, err := client.Do(httpRequest)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, res)
		e.logger.Error("error sending Permit.BulkCheck() request to PDP", zap.Error(permitError))
		return nil, permitError
	}
	results, err := e.parseBulkResponse(res)
	if err != nil {
		return nil, err
	}
	if len(results) != len(requests) {
		return nil, errors.NewPermitUnexpectedError(fmt.Errorf("unexpected number of results"), res)
	}
	allowResults := make([]bool, len(results))
	for result := range results {
		allowResults[result] = results[result].Allow
	}
	return allowResults, nil
}

func (e *PermitEnforcer) FilterObjects(user User, action Action, context map[string]string, resources ...ResourceI) ([]ResourceI, error) {
	requests := make([]CheckRequest, len(resources))
	for i, resource := range resources {
		permitResource := ResourceBuilder(resource.GetType()).
			WithID(resource.GetID()).
			WithContext(resource.GetContext()).
			WithAttributes(resource.GetAttributes()).
			WithTenant(resource.GetTenant()).
			Build()
		requests[i] = *NewCheckRequest(user,
			action,
			permitResource,
			context,
		)
	}
	results, err := e.BulkCheck(requests...)
	if err != nil {
		return nil, err
	}
	filteredResources := make([]ResourceI, 0)
	for i, result := range results {
		if result {
			filteredResources = append(filteredResources, resources[i])
		}
	}

	return filteredResources, nil
}
