/*
Permit.io API

 Authorization as a service

API version: 2.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"bytes"
	"context"
	"github.com/permitio/permit-golang/pkg/models"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

// ConditionSetRulesApiService ConditionSetRulesApi service
type ConditionSetRulesApiService service

type ApiAssignSetPermissionsRequest struct {
	ctx                    context.Context
	ApiService             *ConditionSetRulesApiService
	projId                 string
	envId                  string
	conditionSetRuleCreate *models.ConditionSetRuleCreate
}

func (r ApiAssignSetPermissionsRequest) ConditionSetRuleCreate(conditionSetRuleCreate models.ConditionSetRuleCreate) ApiAssignSetPermissionsRequest {
	r.conditionSetRuleCreate = &conditionSetRuleCreate
	return r
}

func (r ApiAssignSetPermissionsRequest) Execute() ([]models.ConditionSetRuleRead, *http.Response, error) {
	return r.ApiService.AssignSetPermissionsExecute(r)
}

/*
AssignSetPermissions Assign Set Permissions

Grant permissions to a user set *on* a resource set.

If the permission is already granted, it is skipped.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@return ApiAssignSetPermissionsRequest
*/
func (a *ConditionSetRulesApiService) AssignSetPermissions(ctx context.Context, projId string, envId string) ApiAssignSetPermissionsRequest {
	return ApiAssignSetPermissionsRequest{
		ApiService: a,
		ctx:        ctx,
		projId:     projId,
		envId:      envId,
	}
}

// Execute executes the request
//
//	@return []ConditionSetRuleRead
func (a *ConditionSetRulesApiService) AssignSetPermissionsExecute(r ApiAssignSetPermissionsRequest) ([]models.ConditionSetRuleRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodPost
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue []models.ConditionSetRuleRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetRulesApiService.AssignSetPermissions")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/set_rules"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.conditionSetRuleCreate == nil {
		return localVarReturnValue, nil, reportError("conditionSetRuleCreate is required and must be specified")
	}

	// to determine the Content-Type header
	localVarHTTPContentTypes := []string{"application/json"}

	// set Content-Type header
	localVarHTTPContentType := selectHeaderContentType(localVarHTTPContentTypes)
	if localVarHTTPContentType != "" {
		localVarHeaderParams["Content-Type"] = localVarHTTPContentType
	}

	// to determine the Accept header
	localVarHTTPHeaderAccepts := []string{"application/json"}

	// set Accept header
	localVarHTTPHeaderAccept := selectHeaderAccept(localVarHTTPHeaderAccepts)
	if localVarHTTPHeaderAccept != "" {
		localVarHeaderParams["Accept"] = localVarHTTPHeaderAccept
	}
	// body params
	localVarPostBody = r.conditionSetRuleCreate
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return localVarReturnValue, nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := ioutil.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = ioutil.NopCloser(bytes.NewBuffer(localVarBody))
	if err != nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	if localVarHTTPResponse.StatusCode >= 300 {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: localVarHTTPResponse.Status,
		}
		if localVarHTTPResponse.StatusCode == 422 {
			var v models.HTTPValidationError
			err = a.client.decode(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
			if err != nil {
				newErr.error = err.Error()
				return localVarReturnValue, localVarHTTPResponse, newErr
			}
			newErr.error = formatErrorMessage(localVarHTTPResponse.Status, &v)
			newErr.model = v
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	err = a.client.decode(&localVarReturnValue, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
	if err != nil {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: err.Error(),
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	return localVarReturnValue, localVarHTTPResponse, nil
}

type ApiListSetPermissionsRequest struct {
	ctx         context.Context
	ApiService  *ConditionSetRulesApiService
	projId      string
	envId       string
	userSet     *string
	permission  *string
	resourceSet *string
	page        *int32
	perPage     *int32
}

// optional user set filter, will only return rules where the permission is granted to this user set
func (r ApiListSetPermissionsRequest) UserSet(userSet string) ApiListSetPermissionsRequest {
	r.userSet = &userSet
	return r
}

// optional permission filter, will only return condition set rules granting this permission
func (r ApiListSetPermissionsRequest) Permission(permission string) ApiListSetPermissionsRequest {
	r.permission = &permission
	return r
}

// optional resource set filter, will only return rules where the permission is granted on this resource set
func (r ApiListSetPermissionsRequest) ResourceSet(resourceSet string) ApiListSetPermissionsRequest {
	r.resourceSet = &resourceSet
	return r
}

// Page number of the results to fetch, starting at 1.
func (r ApiListSetPermissionsRequest) Page(page int32) ApiListSetPermissionsRequest {
	r.page = &page
	return r
}

// The number of results per page (max 100).
func (r ApiListSetPermissionsRequest) PerPage(perPage int32) ApiListSetPermissionsRequest {
	r.perPage = &perPage
	return r
}

func (r ApiListSetPermissionsRequest) Execute() ([]models.ConditionSetRuleRead, *http.Response, error) {
	return r.ApiService.ListSetPermissionsExecute(r)
}

/*
ListSetPermissions List Set Permissions

Lists the condition set rules matching the filter.
- If the `user_set` filter is present, will only return the permissions set of that user set.
- If the `permission` filter is present, will only return the permissions sets that equals to the queried permission.
- If the `resource_set` filter is present, will only return the permissions set of that resource set.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@return ApiListSetPermissionsRequest
*/
func (a *ConditionSetRulesApiService) ListSetPermissions(ctx context.Context, projId string, envId string) ApiListSetPermissionsRequest {
	return ApiListSetPermissionsRequest{
		ApiService: a,
		ctx:        ctx,
		projId:     projId,
		envId:      envId,
	}
}

// Execute executes the request
//
//	@return []ConditionSetRuleRead
func (a *ConditionSetRulesApiService) ListSetPermissionsExecute(r ApiListSetPermissionsRequest) ([]models.ConditionSetRuleRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodGet
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue []models.ConditionSetRuleRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetRulesApiService.ListSetPermissions")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/set_rules"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.userSet != nil {
		localVarQueryParams.Add("user_set", parameterToString(*r.userSet, ""))
	}
	if r.permission != nil {
		localVarQueryParams.Add("permission", parameterToString(*r.permission, ""))
	}
	if r.resourceSet != nil {
		localVarQueryParams.Add("resource_set", parameterToString(*r.resourceSet, ""))
	}
	if r.page != nil {
		localVarQueryParams.Add("page", parameterToString(*r.page, ""))
	}
	if r.perPage != nil {
		localVarQueryParams.Add("per_page", parameterToString(*r.perPage, ""))
	}
	// to determine the Content-Type header
	localVarHTTPContentTypes := []string{}

	// set Content-Type header
	localVarHTTPContentType := selectHeaderContentType(localVarHTTPContentTypes)
	if localVarHTTPContentType != "" {
		localVarHeaderParams["Content-Type"] = localVarHTTPContentType
	}

	// to determine the Accept header
	localVarHTTPHeaderAccepts := []string{"application/json"}

	// set Accept header
	localVarHTTPHeaderAccept := selectHeaderAccept(localVarHTTPHeaderAccepts)
	if localVarHTTPHeaderAccept != "" {
		localVarHeaderParams["Accept"] = localVarHTTPHeaderAccept
	}
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return localVarReturnValue, nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := ioutil.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = ioutil.NopCloser(bytes.NewBuffer(localVarBody))
	if err != nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	if localVarHTTPResponse.StatusCode >= 300 {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: localVarHTTPResponse.Status,
		}
		if localVarHTTPResponse.StatusCode == 422 {
			var v models.HTTPValidationError
			err = a.client.decode(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
			if err != nil {
				newErr.error = err.Error()
				return localVarReturnValue, localVarHTTPResponse, newErr
			}
			newErr.error = formatErrorMessage(localVarHTTPResponse.Status, &v)
			newErr.model = v
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	err = a.client.decode(&localVarReturnValue, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
	if err != nil {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: err.Error(),
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	return localVarReturnValue, localVarHTTPResponse, nil
}

type ApiUnassignSetPermissionsRequest struct {
	ctx                    context.Context
	ApiService             *ConditionSetRulesApiService
	projId                 string
	envId                  string
	conditionSetRuleRemove *models.ConditionSetRuleRemove
}

func (r ApiUnassignSetPermissionsRequest) ConditionSetRuleRemove(conditionSetRuleRemove models.ConditionSetRuleRemove) ApiUnassignSetPermissionsRequest {
	r.conditionSetRuleRemove = &conditionSetRuleRemove
	return r
}

func (r ApiUnassignSetPermissionsRequest) Execute() (*http.Response, error) {
	return r.ApiService.UnassignSetPermissionsExecute(r)
}

/*
UnassignSetPermissions Unassign Set Permissions

Revokes permissions to a user set *on* a resource set.

If the permission is not granted, it is skipped.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@return ApiUnassignSetPermissionsRequest
*/
func (a *ConditionSetRulesApiService) UnassignSetPermissions(ctx context.Context, projId string, envId string) ApiUnassignSetPermissionsRequest {
	return ApiUnassignSetPermissionsRequest{
		ApiService: a,
		ctx:        ctx,
		projId:     projId,
		envId:      envId,
	}
}

// Execute executes the request
func (a *ConditionSetRulesApiService) UnassignSetPermissionsExecute(r ApiUnassignSetPermissionsRequest) (*http.Response, error) {
	var (
		localVarHTTPMethod = http.MethodDelete
		localVarPostBody   interface{}
		formFiles          []formFile
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetRulesApiService.UnassignSetPermissions")
	if err != nil {
		return nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/set_rules"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.conditionSetRuleRemove == nil {
		return nil, reportError("conditionSetRuleRemove is required and must be specified")
	}

	// to determine the Content-Type header
	localVarHTTPContentTypes := []string{"application/json"}

	// set Content-Type header
	localVarHTTPContentType := selectHeaderContentType(localVarHTTPContentTypes)
	if localVarHTTPContentType != "" {
		localVarHeaderParams["Content-Type"] = localVarHTTPContentType
	}

	// to determine the Accept header
	localVarHTTPHeaderAccepts := []string{"application/json"}

	// set Accept header
	localVarHTTPHeaderAccept := selectHeaderAccept(localVarHTTPHeaderAccepts)
	if localVarHTTPHeaderAccept != "" {
		localVarHeaderParams["Accept"] = localVarHTTPHeaderAccept
	}
	// body params
	localVarPostBody = r.conditionSetRuleRemove
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarHTTPResponse, err
	}

	localVarBody, err := ioutil.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = ioutil.NopCloser(bytes.NewBuffer(localVarBody))
	if err != nil {
		return localVarHTTPResponse, err
	}

	if localVarHTTPResponse.StatusCode >= 300 {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: localVarHTTPResponse.Status,
		}
		if localVarHTTPResponse.StatusCode == 422 {
			var v models.HTTPValidationError
			err = a.client.decode(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
			if err != nil {
				newErr.error = err.Error()
				return localVarHTTPResponse, newErr
			}
			newErr.error = formatErrorMessage(localVarHTTPResponse.Status, &v)
			newErr.model = v
		}
		return localVarHTTPResponse, newErr
	}

	return localVarHTTPResponse, nil
}
