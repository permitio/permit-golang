package enforcement

import (
	"bytes"
	"encoding/json"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type TenantDetails struct {
	Key        string                 `json:"key"`
	Attributes map[string]interface{} `json:"attributes"`
}

type AllTenantsCheckResponse struct {
	CheckResponse
	Tenant TenantDetails `json:"tenant"`
}

type opaAllTenantsResponse struct {
	Result *allowedTenantsResponse `json:"result"`
}

type allowedTenantsResponse struct {
	AllowedTenants *[]AllTenantsCheckResponse `json:"allowed_tenants"`
}

func (e *PermitEnforcer) getAllTenantsCheckEndpoint() string {
	return e.getEndpointByPolicyPackage(allTenantsPolicyPackage)
}

func (e *PermitEnforcer) parseAllTenantsResponse(res *http.Response) ([]AllTenantsCheckResponse, error) {
	var result []AllTenantsCheckResponse
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, res)
		e.logger.Error("error reading Permit.AllTenantsCheck() response from PDP", zap.Error(permitError))
		return nil, permitError
	}
	err = errors.HttpErrorHandle(err, res)
	if err != nil {
		e.logger.Error(string(bodyBytes), zap.Error(err))
		return nil, err
	}
	if e.config.GetOpaUrl() != "" {
		opaStruct := &opaAllTenantsResponse{
			Result: &allowedTenantsResponse{
				&result,
			},
		}

		if err := json.Unmarshal(bodyBytes, opaStruct); err != nil {
			permitError := errors.NewPermitUnexpectedError(err, res)
			e.logger.Error("error unmarshalling Permit.AllTenantsCheck() response from OPA", zap.Error(permitError))
			return nil, err
		}
	} else {
		pdpStruct := &allowedTenantsResponse{&result}
		if err := json.Unmarshal(bodyBytes, &pdpStruct); err != nil {
			permitError := errors.NewPermitUnexpectedError(err, res)
			e.logger.Error("error unmarshalling Permit.AllTenantsCheck() response from PDP", zap.Error(permitError))
			return nil, permitError
		}
	}

	return result, nil
}

func (e *PermitEnforcer) AllTenantsCheck(user User, action Action, resource Resource, additionalContext ...map[string]string) ([]TenantDetails, error) {
	reqAuthValue := "Bearer " + e.config.GetToken()

	if additionalContext == nil {
		additionalContext = make([]map[string]string, 0)
		additionalContext = append(additionalContext, make(map[string]string))
	}
	jsonCheckReq, err := newJsonCheckRequest(e.config.GetOpaUrl(), user, action, resource, additionalContext[0])
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error marshalling Permit.AllTenantsCheck() request", zap.Error(permitError))
		return nil, permitError
	}
	reqBody := bytes.NewBuffer(jsonCheckReq)
	httpRequest, err := http.NewRequest(reqMethod, e.getAllTenantsCheckEndpoint(), reqBody)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error creating Permit.AllTenantsCheck() request", zap.Error(permitError))
		return nil, permitError
	}
	httpRequest.Header.Set(reqContentTypeKey, reqContentTypeValue)
	httpRequest.Header.Set(reqAuthKey, reqAuthValue)
	res, err := client.Do(httpRequest)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, res)
		e.logger.Error("error sending Permit.AllTenantsCheck() request to PDP", zap.Error(permitError))
		return nil, permitError
	}
	results, err := e.parseAllTenantsResponse(res)
	if err != nil {
		return nil, err
	}
	allowResults := make([]TenantDetails, len(results))
	for result := range results {
		allowResults[result] = results[result].Tenant
	}
	return allowResults, nil
}
