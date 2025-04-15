package enforcement

import (
	"bytes"
	"encoding/json"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type TenantUserPermissions struct {
	Tenant      TenantDetails `json:"tenant"`
	Resource    *Resource     `json:"resource,omitempty"`
	Permissions []string      `json:"permissions"`
	Roles       []string      `json:"roles"`
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
	User    User              `json:"user"`
	Tenants []string          `json:"tenants,omitempty"`
	Context map[string]string `json:"context,omitempty"`
}

func NewGetUserPermissionsRequest(user User, tenants []string) *GetUserPermissionsRequest {
	return &GetUserPermissionsRequest{
		User:    user,
		Tenants: tenants,
		Context: nil,
	}
}

func newJsonGetUserPermissionsRequest(opaUrl string, user User, tenants []string) ([]byte, error) {
	getUserPermissionsReq := NewGetUserPermissionsRequest(user, tenants)
	var genericCheckReq interface{} = getUserPermissionsReq
	if opaUrl != "" {
		genericCheckReq = &struct {
			Input *GetUserPermissionsRequest `json:"input"`
		}{getUserPermissionsReq}
	}
	jsonCheckReq, err := json.Marshal(genericCheckReq)
	if err != nil {
		return nil, err
	}
	return jsonCheckReq, nil
}

func (e *PermitEnforcer) GetUserPermissions(user User, tenants ...string) (UserPermissions, error) {
	reqAuthValue := "Bearer " + e.config.GetToken()

	jsonCheckReq, err := newJsonGetUserPermissionsRequest(e.config.GetOpaUrl(), user, tenants)
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
