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

// ConditionSetsApiService ConditionSetsApi service
type ConditionSetsApiService service

type ApiCreateConditionSetRequest struct {
	ctx                context.Context
	ApiService         *ConditionSetsApiService
	projId             string
	envId              string
	conditionSetCreate *models.ConditionSetCreate
}

func (r ApiCreateConditionSetRequest) ConditionSetCreate(conditionSetCreate models.ConditionSetCreate) ApiCreateConditionSetRequest {
	r.conditionSetCreate = &conditionSetCreate
	return r
}

func (r ApiCreateConditionSetRequest) Execute() (*models.ConditionSetRead, *http.Response, error) {
	return r.ApiService.CreateConditionSetExecute(r)
}

/*
CreateConditionSet Create Condition Set

Creates a new condition set (can be either a user set or a resource set).

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @return ApiCreateConditionSetRequest
*/
func (a *ConditionSetsApiService) CreateConditionSet(ctx context.Context, projId string, envId string) ApiCreateConditionSetRequest {
	return ApiCreateConditionSetRequest{
		ApiService: a,
		ctx:        ctx,
		projId:     projId,
		envId:      envId,
	}
}

// Execute executes the request
//  @return ConditionSetRead
func (a *ConditionSetsApiService) CreateConditionSetExecute(r ApiCreateConditionSetRequest) (*models.ConditionSetRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodPost
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue *models.ConditionSetRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetsApiService.CreateConditionSet")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/condition_sets"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.conditionSetCreate == nil {
		return localVarReturnValue, nil, reportError("conditionSetCreate is required and must be specified")
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
	localVarPostBody = r.conditionSetCreate
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

type ApiDeleteConditionSetRequest struct {
	ctx            context.Context
	ApiService     *ConditionSetsApiService
	projId         string
	envId          string
	conditionSetId string
}

func (r ApiDeleteConditionSetRequest) Execute() (*http.Response, error) {
	return r.ApiService.DeleteConditionSetExecute(r)
}

/*
DeleteConditionSet Delete Condition Set

Deletes a condition set and all its related data.
This includes any permissions granted to said condition set (i.e: any matching condition set rules).

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @param conditionSetId Either the unique id of the condition set, or the URL-friendly key of the condition set (i.e: the \"slug\").
 @return ApiDeleteConditionSetRequest
*/
func (a *ConditionSetsApiService) DeleteConditionSet(ctx context.Context, projId string, envId string, conditionSetId string) ApiDeleteConditionSetRequest {
	return ApiDeleteConditionSetRequest{
		ApiService:     a,
		ctx:            ctx,
		projId:         projId,
		envId:          envId,
		conditionSetId: conditionSetId,
	}
}

// Execute executes the request
func (a *ConditionSetsApiService) DeleteConditionSetExecute(r ApiDeleteConditionSetRequest) (*http.Response, error) {
	var (
		localVarHTTPMethod = http.MethodDelete
		localVarPostBody   interface{}
		formFiles          []formFile
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetsApiService.DeleteConditionSet")
	if err != nil {
		return nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"condition_set_id"+"}", url.PathEscape(parameterToString(r.conditionSetId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

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

type ApiGetConditionSetRequest struct {
	ctx            context.Context
	ApiService     *ConditionSetsApiService
	projId         string
	envId          string
	conditionSetId string
}

func (r ApiGetConditionSetRequest) Execute() (*models.ConditionSetRead, *http.Response, error) {
	return r.ApiService.GetConditionSetExecute(r)
}

/*
GetConditionSet Get Condition Set

Gets a single condition set, if such condition set exists.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @param conditionSetId Either the unique id of the condition set, or the URL-friendly key of the condition set (i.e: the \"slug\").
 @return ApiGetConditionSetRequest
*/
func (a *ConditionSetsApiService) GetConditionSet(ctx context.Context, projId string, envId string, conditionSetId string) ApiGetConditionSetRequest {
	return ApiGetConditionSetRequest{
		ApiService:     a,
		ctx:            ctx,
		projId:         projId,
		envId:          envId,
		conditionSetId: conditionSetId,
	}
}

// Execute executes the request
//  @return ConditionSetRead
func (a *ConditionSetsApiService) GetConditionSetExecute(r ApiGetConditionSetRequest) (*models.ConditionSetRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodGet
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue *models.ConditionSetRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetsApiService.GetConditionSet")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"condition_set_id"+"}", url.PathEscape(parameterToString(r.conditionSetId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

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

type ApiListConditionSetsRequest struct {
	ctx        context.Context
	ApiService *ConditionSetsApiService
	projId     string
	envId      string
	type_      *models.ConditionSetType
	page       *int32
	perPage    *int32
}

// if provided, will return only the condition sets of the specified type. e.g: only user sets.
func (r ApiListConditionSetsRequest) Type_(type_ models.ConditionSetType) ApiListConditionSetsRequest {
	r.type_ = &type_
	return r
}

// Page number of the results to fetch, starting at 1.
func (r ApiListConditionSetsRequest) Page(page int32) ApiListConditionSetsRequest {
	r.page = &page
	return r
}

// The number of results per page (max 100).
func (r ApiListConditionSetsRequest) PerPage(perPage int32) ApiListConditionSetsRequest {
	r.perPage = &perPage
	return r
}

func (r ApiListConditionSetsRequest) Execute() ([]models.ConditionSetRead, *http.Response, error) {
	return r.ApiService.ListConditionSetsExecute(r)
}

/*
ListConditionSets List Condition Sets

Lists all condition sets matching a filter.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @return ApiListConditionSetsRequest
*/
func (a *ConditionSetsApiService) ListConditionSets(ctx context.Context, projId string, envId string) ApiListConditionSetsRequest {
	return ApiListConditionSetsRequest{
		ApiService: a,
		ctx:        ctx,
		projId:     projId,
		envId:      envId,
	}
}

// Execute executes the request
//  @return []ConditionSetRead
func (a *ConditionSetsApiService) ListConditionSetsExecute(r ApiListConditionSetsRequest) ([]models.ConditionSetRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodGet
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue []models.ConditionSetRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetsApiService.ListConditionSets")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/condition_sets"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.type_ != nil {
		localVarQueryParams.Add("type", parameterToString(*r.type_, ""))
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

type ApiUpdateConditionSetRequest struct {
	ctx                context.Context
	ApiService         *ConditionSetsApiService
	projId             string
	envId              string
	conditionSetId     string
	conditionSetUpdate *models.ConditionSetUpdate
}

func (r ApiUpdateConditionSetRequest) ConditionSetUpdate(conditionSetUpdate models.ConditionSetUpdate) ApiUpdateConditionSetRequest {
	r.conditionSetUpdate = &conditionSetUpdate
	return r
}

func (r ApiUpdateConditionSetRequest) Execute() (*models.ConditionSetRead, *http.Response, error) {
	return r.ApiService.UpdateConditionSetExecute(r)
}

/*
UpdateConditionSet Update Condition Set

Partially updates a condition set.
Fields that will be provided will be completely overwritten.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @param conditionSetId Either the unique id of the condition set, or the URL-friendly key of the condition set (i.e: the \"slug\").
 @return ApiUpdateConditionSetRequest
*/
func (a *ConditionSetsApiService) UpdateConditionSet(ctx context.Context, projId string, envId string, conditionSetId string) ApiUpdateConditionSetRequest {
	return ApiUpdateConditionSetRequest{
		ApiService:     a,
		ctx:            ctx,
		projId:         projId,
		envId:          envId,
		conditionSetId: conditionSetId,
	}
}

// Execute executes the request
//  @return ConditionSetRead
func (a *ConditionSetsApiService) UpdateConditionSetExecute(r ApiUpdateConditionSetRequest) (*models.ConditionSetRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodPatch
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue *models.ConditionSetRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ConditionSetsApiService.UpdateConditionSet")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"condition_set_id"+"}", url.PathEscape(parameterToString(r.conditionSetId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.conditionSetUpdate == nil {
		return localVarReturnValue, nil, reportError("conditionSetUpdate is required and must be specified")
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
	localVarPostBody = r.conditionSetUpdate
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
