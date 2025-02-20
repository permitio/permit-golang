package enforcement

import (
	"bytes"
	"encoding/json"
	"errors"
	pErr "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/log"
	"io"
	"net/http"
)

type Action string

type CheckResponse struct {
	Allow  bool                   `json:"allow"`
	Query  map[string]interface{} `json:"query"`
	Debug  map[string]interface{} `json:"debug"`
	Result bool                   `json:"result"`
}

type CheckRequest struct {
	User     User              `json:"user"`
	Action   Action            `json:"action"`
	Resource Resource          `json:"resource"`
	Context  map[string]string `json:"context"`
}

func NewCheckRequest(user User, action Action, resource Resource, context map[string]string) *CheckRequest {
	return &CheckRequest{
		User:     user,
		Action:   action,
		Resource: resource,
		Context:  context,
	}
}

func newJsonCheckRequest(opaUrl string, user User, action Action, resource Resource, context map[string]string) ([]byte, error) {
	checkReq := NewCheckRequest(user, action, resource, context)
	var genericCheckReq interface{} = checkReq
	if opaUrl != "" {
		genericCheckReq = &struct {
			Input *CheckRequest `json:"input"`
		}{checkReq}
	}
	jsonCheckReq, err := json.Marshal(genericCheckReq)
	if err != nil {
		return nil, err
	}
	return jsonCheckReq, nil
}

func (e *PermitEnforcer) getCheckEndpoint() string {
	return e.getEndpointByPolicyPackage(mainPolicyPackage)
}

func (e *PermitEnforcer) parseResponse(res *http.Response) (*CheckResponse, error) {
	var result CheckResponse
	err := pErr.HttpErrorHandle(nil, res)
	if err != nil {
		responseBody := ""
		var permitErr pErr.PermitError
		if errors.As(err, &permitErr) {
			responseBody = permitErr.ResponseBody
		}
		e.logger.Error("erroneous http response from PDP for Permit.Check()", err, log.ResponseBodyKey, responseBody)
		return nil, err
	}

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		permitError := pErr.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error reading Permit.Check() response from PDP", permitError)
		return nil, permitError
	}

	if e.config.GetOpaUrl() != "" {
		opaStruct := &struct {
			Result *CheckResponse `json:"result"`
		}{&result}

		if err := json.Unmarshal(bodyBytes, opaStruct); err != nil {
			permitError := pErr.NewPermitUnexpectedError(err, nil)
			e.logger.Error("error unmarshalling Permit.Check() response from OPA", permitError)
			return nil, err
		}
	} else {
		if err := json.Unmarshal(bodyBytes, &result); err != nil {
			permitError := pErr.NewPermitUnexpectedError(err, nil)
			e.logger.Error("error unmarshalling Permit.Check response from PDP", permitError)
			return nil, permitError
		}
	}

	return &result, nil
}

func (e *PermitEnforcer) Check(user User, action Action, resource Resource, additionalContext ...map[string]string) (bool, error) {
	reqAuthValue := "Bearer " + e.config.GetToken()

	if additionalContext == nil {
		additionalContext = make([]map[string]string, 0)
		additionalContext = append(additionalContext, make(map[string]string))
	}
	jsonCheckReq, err := newJsonCheckRequest(e.config.GetOpaUrl(), user, action, resource, additionalContext[0])
	if err != nil {
		permitError := pErr.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error marshalling Permit.Check() request", permitError)
		return false, permitError
	}
	reqBody := bytes.NewBuffer(jsonCheckReq)
	httpRequest, err := http.NewRequest(reqMethod, e.getCheckEndpoint(), reqBody)
	if err != nil {
		permitError := pErr.NewPermitUnexpectedError(err, nil)
		e.logger.Error("error creating Permit.Check() request", permitError)
		return false, permitError
	}
	httpRequest.Header.Set(reqContentTypeKey, reqContentTypeValue)
	httpRequest.Header.Set(reqAuthKey, reqAuthValue)
	res, err := e.client.Do(httpRequest)
	if err != nil {
		permitError := pErr.NewPermitUnexpectedError(err, res)
		e.logger.Error("error sending Permit.Check() request to PDP", permitError)
		return false, permitError
	}
	result, err := e.parseResponse(res)
	if err != nil {
		return false, err
	}
	return result.Allow, nil
}
