package enforcement

import (
	"bytes"
	"encoding/json"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type ResourceDetails struct {
	Type       string                 `json:"type,omitempty"`
	Key        string                 `json:"key,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type TenantUserPermissions struct {
	Tenant      TenantDetails    `json:"tenant"`
	Resource    *ResourceDetails `json:"resource,omitempty"`
	Permissions []string         `json:"permissions"`
	Roles       []string         `json:"roles"`
}

type UserPermissions map[string]TenantUserPermissions

type userPermissionsResponse struct {
	Permissions *UserPermissions `json:"permissions"`
}

type opaUserPermissionsResponse struct {
	Result *userPermissionsResponse `json:"result"`
}

func (e *PermitEnforcer) getUserPermissionsEndpoint() string {
	return e.getEndpointByPolicyPackage(userPermissionsPackage)
}

func (e *PermitEnforcer) parseUserPermissionsResponse(res *http.Response) (UserPermissions, error) {
	var result UserPermissions
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, res)
		e.logger.Error("error reading Permit.GetUserPermissions() response from PDP", zap.Error(permitError))
		return nil, permitError
	}
	err = errors.HttpErrorHandle(err, res)
	if err != nil {
		e.logger.Error(string(bodyBytes), zap.Error(err))
		return nil, err
	}
	if e.config.GetOpaUrl() != "" {
		opaStruct := &opaUserPermissionsResponse{
			Result: &userPermissionsResponse{
				&result,
			},
		}

		if err := json.Unmarshal(bodyBytes, opaStruct); err != nil {
			permitError := errors.NewPermitUnexpectedError(err, res)
			e.logger.Error("error unmarshalling Permit.GetUserPermissions() response from OPA", zap.Error(permitError))
			return nil, err
		}
	} else {
		pdpStruct := &userPermissionsResponse{&result}
		if err := json.Unmarshal(bodyBytes, &pdpStruct.Permissions); err != nil {
			permitError := errors.NewPermitUnexpectedError(err, res)
			e.logger.Error("error unmarshalling Permit.GetUserPermissions() response from PDP", zap.Error(permitError))
			return nil, permitError
		}
	}

	return result, nil
}

type GetUserPermissionsRequest struct {
	User          User              `json:"user"`
	Tenants       []string          `json:"tenants,omitempty"`
	ResourceTypes []string          `json:"resource_types,omitempty"`
	Resources     []string          `json:"resources,omitempty"`
	Context       map[string]string `json:"context,omitempty"`
}

// UserPermissionOption is a function that modifies GetUserPermissionsRequest
type UserPermissionOption func(*GetUserPermissionsRequest)

// WithTenants adds tenant filters to the user permissions request
func WithTenants(tenants []string) UserPermissionOption {
	return func(r *GetUserPermissionsRequest) {
		r.Tenants = tenants
	}
}

// WithResourceTypes adds resource type filters to the user permissions request
func WithResourceTypes(resourceTypes []string) UserPermissionOption {
	return func(r *GetUserPermissionsRequest) {
		r.ResourceTypes = resourceTypes
	}
}

// WithResources adds resource instance filters to the user permissions request
func WithResources(resources []string) UserPermissionOption {
	return func(r *GetUserPermissionsRequest) {
		r.Resources = resources
	}
}

// WithContext adds context data to the user permissions request
func WithContext(context map[string]interface{}) UserPermissionOption {
	return func(r *GetUserPermissionsRequest) {
		// Convert the map[string]interface{} to map[string]string for compatibility
		if context != nil {
			contextMap := make(map[string]string)
			for k, v := range context {
				// Convert value to string if it's not already
				switch val := v.(type) {
				case string:
					contextMap[k] = val
				case bool:
					if val {
						contextMap[k] = "true"
					} else {
						contextMap[k] = "false"
					}
				default:
					// Try to marshal other types to JSON
					jsonBytes, err := json.Marshal(v)
					if err == nil {
						contextMap[k] = string(jsonBytes)
					}
				}
			}
			r.Context = contextMap
		}
	}
}

// GetUserPermissions returns permissions a user has, optionally filtered by tenants, resource types, resources, and with custom context
func (e *PermitEnforcer) GetUserPermissions(user User, opts ...UserPermissionOption) (UserPermissions, error) {
	// Create base request with just the required user
	req := &GetUserPermissionsRequest{
		User: user,
	}
	
	// Apply all options
	for _, opt := range opts {
		opt(req)
	}
	
	// Build request and send to API
	reqAuthValue := "Bearer " + e.config.GetToken()
	
	var genericCheckReq interface{} = req
	if e.config.GetOpaUrl() != "" {
		genericCheckReq = &struct {
			Input *GetUserPermissionsRequest `json:"input"`
		}{req}
	}
	
	jsonCheckReq, err := json.Marshal(genericCheckReq)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error marshalling Permit.GetUserPermissions() request", zap.Error(permitError))
		return nil, permitError
	}
	
	reqBody := bytes.NewBuffer(jsonCheckReq)
	httpRequest, err := http.NewRequest(reqMethod, e.getUserPermissionsEndpoint(), reqBody)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error creating Permit.GetUserPermissions() request", zap.Error(permitError))
		return nil, permitError
	}
	
	httpRequest.Header.Set(reqContentTypeKey, reqContentTypeValue)
	httpRequest.Header.Set(reqAuthKey, reqAuthValue)
	res, err := e.client.Do(httpRequest)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err, res)
		e.logger.Error("error sending Permit.GetUserPermissions() request to PDP", zap.Error(permitError))
		return nil, permitError
	}
	
	result, err := e.parseUserPermissionsResponse(res)
	if err != nil {
		return nil, err
	}
	
	return result, nil
}
