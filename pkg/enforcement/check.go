package enforcement

import (
	"bytes"
	"encoding/json"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type Action string

type opaCheckRequest struct {
	Input *CheckRequest `json:"input"`
}

type AuthorizationResult struct {
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
	var jsonCheckReq []byte
	var err error
	if opaUrl != "" {
		opaCheckReq := &opaCheckRequest{
			checkReq,
		}
		jsonCheckReq, err = json.Marshal(opaCheckReq)
	} else {
		jsonCheckReq, err = json.Marshal(checkReq)
	}
	if err != nil {
		return nil, err
	}
	return jsonCheckReq, nil
}

func (e *PermitEnforcer) getCheckEndpoint() string {
	if opaUrl := e.config.GetOpaUrl(); opaUrl != "" {
		return opaUrl + "/v1/data/" + mainPolicyPath
	} else {
		return e.config.GetPdpUrl() + "/allowed"
	}
}

func (e *PermitEnforcer) parseResponse(res *http.Response) (*AuthorizationResult, error) {
	var result AuthorizationResult
	var resMap map[string]interface{}
	err := json.NewDecoder(res.Body).Decode(&resMap)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err)
		e.logger.Error("error decoding Permit.Check() response from PDP", zap.Error(permitError))
		return nil, permitError
	}
	resBytes, err := json.Marshal(resMap)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err)
		e.logger.Error("error marshalling Permit.Check() response from PDP", zap.Error(permitError))
		return nil, permitError
	}
	err = errors.HttpErrorHandle(err, res)
	if err != nil {
		e.logger.Error(string(resBytes), zap.Error(err))
		return nil, err
	}
	resultBytes := resBytes
	if e.config.GetOpaUrl() != "" {
		resultBytes, err = json.Marshal(resMap[resultKey])
		if err != nil {
			permitError := errors.NewPermitUnexpectedError(err)
			e.logger.Error("error marshalling Permit.Check() response from PDP", zap.Error(permitError))
			return nil, err
		}
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		permitError := errors.NewPermitUnexpectedError(err)
		e.logger.Error("error unmarshalling Permit.Check response from PDP", zap.Error(permitError))
		return nil, permitError
	}
	return &result, nil
}

func (e *PermitEnforcer) Check(user User, action Action, resource Resource, additionalContext ...map[string]string) (bool, error) {
	const (
		reqMethod           = "POST"
		reqContentTypeKey   = "Content-Type"
		reqContentTypeValue = "application/json"
		reqAuthKey          = "Authorization"
	)
	reqAuthValue := "Bearer " + e.config.GetToken()

	if additionalContext == nil {
		additionalContext = make([]map[string]string, 0)
		additionalContext = append(additionalContext, make(map[string]string))
	}
	jsonCheckReq, err := newJsonCheckRequest(e.config.GetOpaUrl(), user, action, resource, additionalContext[0])
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err)
		e.logger.Error("error marshalling Permit.Check() request", zap.Error(permitError))
		return false, permitError
	}
	reqBody := bytes.NewBuffer(jsonCheckReq)
	httpRequest, err := http.NewRequest(reqMethod, e.getCheckEndpoint(), reqBody)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err)
		e.logger.Error("error creating Permit.Check() request", zap.Error(permitError))
		return false, permitError
	}
	httpRequest.Header.Set(reqContentTypeKey, reqContentTypeValue)
	httpRequest.Header.Set(reqAuthKey, reqAuthValue)
	client := &http.Client{
		Timeout: DefaultTimeout * time.Second,
	}
	res, err := client.Do(httpRequest)
	if err != nil {
		permitError := errors.NewPermitUnexpectedError(err)
		e.logger.Error("error sending Permit.Check() request to PDP", zap.Error(permitError))
		return false, permitError
	}
	result, err := e.parseResponse(res)
	if err != nil {
		return false, err
	}
	return result.Allow, nil
}
