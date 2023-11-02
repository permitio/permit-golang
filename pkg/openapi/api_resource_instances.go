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
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ResourceInstancesApiService ResourceInstancesAPI service
type ResourceInstancesApiService service

type ApiCreateResourceInstanceRequest struct {
	ctx context.Context
	ApiService *ResourceInstancesApiService
	projId string
	envId string
	resourceInstanceCreate *models.ResourceInstanceCreate
	permitSession *string
}

func (r ApiCreateResourceInstanceRequest) ResourceInstanceCreate(resourceInstanceCreate models.ResourceInstanceCreate) ApiCreateResourceInstanceRequest {
	r.resourceInstanceCreate = &resourceInstanceCreate
	return r
}

func (r ApiCreateResourceInstanceRequest) PermitSession(permitSession string) ApiCreateResourceInstanceRequest {
	r.permitSession = &permitSession
	return r
}

func (r ApiCreateResourceInstanceRequest) Execute() (*models.ResourceInstanceRead, *http.Response, error) {
	return r.ApiService.CreateResourceInstanceExecute(r)
}

/*
CreateResourceInstance Create Resource Instance

Creates a new instance inside the Permit.io system.

If the instance is already created: will return 200 instead of 201,
and will return the existing instance object in the response body.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @return ApiCreateResourceInstanceRequest
*/
func (a *ResourceInstancesApiService) CreateResourceInstance(ctx context.Context, projId string, envId string) ApiCreateResourceInstanceRequest {
	return ApiCreateResourceInstanceRequest{
		ApiService: a,
		ctx: ctx,
		projId: projId,
		envId: envId,
	}
}

// Execute executes the request
//  @return ResourceInstanceRead
func (a *ResourceInstancesApiService) CreateResourceInstanceExecute(r ApiCreateResourceInstanceRequest) (*models.ResourceInstanceRead, *http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodPost
		localVarPostBody     interface{}
		formFiles            []formFile
		localVarReturnValue  *models.ResourceInstanceRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ResourceInstancesApiService.CreateResourceInstance")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/resource_instances"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterValueToString(r.projId, "projId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterValueToString(r.envId, "envId")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.resourceInstanceCreate == nil {
		return localVarReturnValue, nil, reportError("resourceInstanceCreate is required and must be specified")
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
	localVarPostBody = r.resourceInstanceCreate
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return localVarReturnValue, nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
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

type ApiDeleteResourceInstanceRequest struct {
	ctx context.Context
	ApiService *ResourceInstancesApiService
	projId string
	envId string
	instanceId string
	permitSession *string
}

func (r ApiDeleteResourceInstanceRequest) PermitSession(permitSession string) ApiDeleteResourceInstanceRequest {
	r.permitSession = &permitSession
	return r
}

func (r ApiDeleteResourceInstanceRequest) Execute() (*http.Response, error) {
	return r.ApiService.DeleteResourceInstanceExecute(r)
}

/*
DeleteResourceInstance Delete Resource Instance

Deletes the instance and all its related data.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @param instanceId Either the unique id of the resource instance, or the URL-friendly key of the resource instance (i.e: the \"slug\").
 @return ApiDeleteResourceInstanceRequest
*/
func (a *ResourceInstancesApiService) DeleteResourceInstance(ctx context.Context, projId string, envId string, instanceId string) ApiDeleteResourceInstanceRequest {
	return ApiDeleteResourceInstanceRequest{
		ApiService: a,
		ctx: ctx,
		projId: projId,
		envId: envId,
		instanceId: instanceId,
	}
}

// Execute executes the request
func (a *ResourceInstancesApiService) DeleteResourceInstanceExecute(r ApiDeleteResourceInstanceRequest) (*http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodDelete
		localVarPostBody     interface{}
		formFiles            []formFile
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ResourceInstancesApiService.DeleteResourceInstance")
	if err != nil {
		return nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterValueToString(r.projId, "projId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterValueToString(r.envId, "envId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"instance_id"+"}", url.PathEscape(parameterValueToString(r.instanceId, "instanceId")), -1)

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

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
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

type ApiGetResourceInstanceRequest struct {
	ctx context.Context
	ApiService *ResourceInstancesApiService
	projId string
	envId string
	instanceId string
	permitSession *string
}

func (r ApiGetResourceInstanceRequest) PermitSession(permitSession string) ApiGetResourceInstanceRequest {
	r.permitSession = &permitSession
	return r
}

func (r ApiGetResourceInstanceRequest) Execute() (*models.ResourceInstanceRead, *http.Response, error) {
	return r.ApiService.GetResourceInstanceExecute(r)
}

/*
GetResourceInstance Get Resource Instance

Gets a instance, if such instance exists. Otherwise returns 404.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @param instanceId Either the unique id of the resource instance, or the URL-friendly key of the resource instance (i.e: the \"slug\").
 @return ApiGetResourceInstanceRequest
*/
func (a *ResourceInstancesApiService) GetResourceInstance(ctx context.Context, projId string, envId string, instanceId string) ApiGetResourceInstanceRequest {
	return ApiGetResourceInstanceRequest{
		ApiService: a,
		ctx: ctx,
		projId: projId,
		envId: envId,
		instanceId: instanceId,
	}
}

// Execute executes the request
//  @return ResourceInstanceRead
func (a *ResourceInstancesApiService) GetResourceInstanceExecute(r ApiGetResourceInstanceRequest) (*models.ResourceInstanceRead, *http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodGet
		localVarPostBody     interface{}
		formFiles            []formFile
		localVarReturnValue  *models.ResourceInstanceRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ResourceInstancesApiService.GetResourceInstance")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterValueToString(r.projId, "projId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterValueToString(r.envId, "envId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"instance_id"+"}", url.PathEscape(parameterValueToString(r.instanceId, "instanceId")), -1)

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

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
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

type ApiListResourceInstancesRequest struct {
	ctx context.Context
	ApiService *ResourceInstancesApiService
	projId string
	envId string
	tenant *string
	resource *string
	detailed *bool
	page *int32
	perPage *int32
	search *string
	permitSession *string
}

// The tenant key or id to filter by
func (r ApiListResourceInstancesRequest) Tenant(tenant string) ApiListResourceInstancesRequest {
	r.tenant = &tenant
	return r
}

// The resource key or id to filter by
func (r ApiListResourceInstancesRequest) Resource(resource string) ApiListResourceInstancesRequest {
	r.resource = &resource
	return r
}

// If true, will return the relationships of the resource instance.
func (r ApiListResourceInstancesRequest) Detailed(detailed bool) ApiListResourceInstancesRequest {
	r.detailed = &detailed
	return r
}

// Page number of the results to fetch, starting at 1.
func (r ApiListResourceInstancesRequest) Page(page int32) ApiListResourceInstancesRequest {
	r.page = &page
	return r
}

// The number of results per page (max 100).
func (r ApiListResourceInstancesRequest) PerPage(perPage int32) ApiListResourceInstancesRequest {
	r.perPage = &perPage
	return r
}

// Text search for the object name or key
func (r ApiListResourceInstancesRequest) Search(search string) ApiListResourceInstancesRequest {
	r.search = &search
	return r
}

func (r ApiListResourceInstancesRequest) PermitSession(permitSession string) ApiListResourceInstancesRequest {
	r.permitSession = &permitSession
	return r
}

func (r ApiListResourceInstancesRequest) Execute() ([]models.ResourceInstanceRead, *http.Response, error) {
	return r.ApiService.ListResourceInstancesExecute(r)
}

/*
ListResourceInstances List Resource Instances

Lists all the resource instances defined within an environment.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @return ApiListResourceInstancesRequest
*/
func (a *ResourceInstancesApiService) ListResourceInstances(ctx context.Context, projId string, envId string) ApiListResourceInstancesRequest {
	return ApiListResourceInstancesRequest{
		ApiService: a,
		ctx: ctx,
		projId: projId,
		envId: envId,
	}
}

// Execute executes the request
//  @return []ResourceInstanceRead
func (a *ResourceInstancesApiService) ListResourceInstancesExecute(r ApiListResourceInstancesRequest) ([]models.ResourceInstanceRead, *http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodGet
		localVarPostBody     interface{}
		formFiles            []formFile
		localVarReturnValue  []models.ResourceInstanceRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ResourceInstancesApiService.ListResourceInstances")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/resource_instances"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterValueToString(r.projId, "projId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterValueToString(r.envId, "envId")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.tenant != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "tenant", r.tenant, "")
	}
	if r.resource != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "resource", r.resource, "")
	}
	if r.detailed != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "detailed", r.detailed, "")
	} else {
		var defaultValue bool = false
		r.detailed = &defaultValue
	}
	if r.page != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "page", r.page, "")
	} else {
		var defaultValue int32 = 1
		r.page = &defaultValue
	}
	if r.perPage != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "per_page", r.perPage, "")
	} else {
		var defaultValue int32 = 30
		r.perPage = &defaultValue
	}
	if r.search != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "search", r.search, "")
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

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
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

type ApiUpdateResourceInstanceRequest struct {
	ctx context.Context
	ApiService *ResourceInstancesApiService
	projId string
	envId string
	instanceId string
	resourceInstanceUpdate *models.ResourceInstanceUpdate
	permitSession *string
}

func (r ApiUpdateResourceInstanceRequest) ResourceInstanceUpdate(resourceInstanceUpdate models.ResourceInstanceUpdate) ApiUpdateResourceInstanceRequest {
	r.resourceInstanceUpdate = &resourceInstanceUpdate
	return r
}

func (r ApiUpdateResourceInstanceRequest) PermitSession(permitSession string) ApiUpdateResourceInstanceRequest {
	r.permitSession = &permitSession
	return r
}

func (r ApiUpdateResourceInstanceRequest) Execute() (*models.ResourceInstanceRead, *http.Response, error) {
	return r.ApiService.UpdateResourceInstanceExecute(r)
}

/*
UpdateResourceInstance Update Resource Instance

Partially updates the instance definition.
Fields that will be provided will be completely overwritten.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
 @param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
 @param instanceId Either the unique id of the resource instance, or the URL-friendly key of the resource instance (i.e: the \"slug\").
 @return ApiUpdateResourceInstanceRequest
*/
func (a *ResourceInstancesApiService) UpdateResourceInstance(ctx context.Context, projId string, envId string, instanceId string) ApiUpdateResourceInstanceRequest {
	return ApiUpdateResourceInstanceRequest{
		ApiService: a,
		ctx: ctx,
		projId: projId,
		envId: envId,
		instanceId: instanceId,
	}
}

// Execute executes the request
//  @return ResourceInstanceRead
func (a *ResourceInstancesApiService) UpdateResourceInstanceExecute(r ApiUpdateResourceInstanceRequest) (*models.ResourceInstanceRead, *http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodPatch
		localVarPostBody     interface{}
		formFiles            []formFile
		localVarReturnValue  *models.ResourceInstanceRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "ResourceInstancesApiService.UpdateResourceInstance")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterValueToString(r.projId, "projId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterValueToString(r.envId, "envId")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"instance_id"+"}", url.PathEscape(parameterValueToString(r.instanceId, "instanceId")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.resourceInstanceUpdate == nil {
		return localVarReturnValue, nil, reportError("resourceInstanceUpdate is required and must be specified")
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
	localVarPostBody = r.resourceInstanceUpdate
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return localVarReturnValue, nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
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
