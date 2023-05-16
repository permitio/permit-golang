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

// UserAttributesApiService UserAttributesApi service
type UserAttributesApiService service

type ApiCreateUserAttributeRequest struct {
	ctx                     context.Context
	ApiService              *UserAttributesApiService
	projId                  string
	envId                   string
	resourceAttributeCreate *models.ResourceAttributeCreate
	resourceId              *string
}

func (r ApiCreateUserAttributeRequest) ResourceAttributeCreate(resourceAttributeCreate models.ResourceAttributeCreate) ApiCreateUserAttributeRequest {
	r.resourceAttributeCreate = &resourceAttributeCreate
	return r
}

func (r ApiCreateUserAttributeRequest) ResourceId(resourceId string) ApiCreateUserAttributeRequest {
	r.resourceId = &resourceId
	return r
}

func (r ApiCreateUserAttributeRequest) Execute() (*models.ResourceAttributeRead, *http.Response, error) {
	return r.ApiService.CreateUserAttributeExecute(r)
}

/*
CreateUserAttribute Create User Attribute

Creates a new attribute for the User resource.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@return ApiCreateUserAttributeRequest
*/
func (a *UserAttributesApiService) CreateUserAttribute(ctx context.Context, projId string, envId string) ApiCreateUserAttributeRequest {
	return ApiCreateUserAttributeRequest{
		ApiService: a,
		ctx:        ctx,
		projId:     projId,
		envId:      envId,
	}
}

// Execute executes the request
//
//	@return ResourceAttributeRead
func (a *UserAttributesApiService) CreateUserAttributeExecute(r ApiCreateUserAttributeRequest) (*models.ResourceAttributeRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodPost
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue *models.ResourceAttributeRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "UserAttributesApiService.CreateUserAttribute")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/users/attributes"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.resourceAttributeCreate == nil {
		return localVarReturnValue, nil, reportError("resourceAttributeCreate is required and must be specified")
	}

	if r.resourceId != nil {
		localVarQueryParams.Add("resource_id", parameterToString(*r.resourceId, ""))
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
	localVarPostBody = r.resourceAttributeCreate
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

type ApiDeleteUserAttributeRequest struct {
	ctx         context.Context
	ApiService  *UserAttributesApiService
	projId      string
	envId       string
	attributeId string
	resourceId  *string
	page        *int32
	perPage     *int32
}

func (r ApiDeleteUserAttributeRequest) ResourceId(resourceId string) ApiDeleteUserAttributeRequest {
	r.resourceId = &resourceId
	return r
}

// Page number of the results to fetch, starting at 1.
func (r ApiDeleteUserAttributeRequest) Page(page int32) ApiDeleteUserAttributeRequest {
	r.page = &page
	return r
}

// The number of results per page (max 100).
func (r ApiDeleteUserAttributeRequest) PerPage(perPage int32) ApiDeleteUserAttributeRequest {
	r.perPage = &perPage
	return r
}

func (r ApiDeleteUserAttributeRequest) Execute() (*http.Response, error) {
	return r.ApiService.DeleteUserAttributeExecute(r)
}

/*
DeleteUserAttribute Delete User Attribute

Deletes the attribute and all its related data.

Note: If the attribute is used by policies, removing it will cause the
attribute to evaluate as `undefined`.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@param attributeId Either the unique id of the attribute, or the URL-friendly key of the attribute (i.e: the \"slug\").
	@return ApiDeleteUserAttributeRequest
*/
func (a *UserAttributesApiService) DeleteUserAttribute(ctx context.Context, projId string, envId string, attributeId string) ApiDeleteUserAttributeRequest {
	return ApiDeleteUserAttributeRequest{
		ApiService:  a,
		ctx:         ctx,
		projId:      projId,
		envId:       envId,
		attributeId: attributeId,
	}
}

// Execute executes the request
func (a *UserAttributesApiService) DeleteUserAttributeExecute(r ApiDeleteUserAttributeRequest) (*http.Response, error) {
	var (
		localVarHTTPMethod = http.MethodDelete
		localVarPostBody   interface{}
		formFiles          []formFile
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "UserAttributesApiService.DeleteUserAttribute")
	if err != nil {
		return nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"attribute_id"+"}", url.PathEscape(parameterToString(r.attributeId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.resourceId != nil {
		localVarQueryParams.Add("resource_id", parameterToString(*r.resourceId, ""))
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

type ApiGetUserAttributeRequest struct {
	ctx         context.Context
	ApiService  *UserAttributesApiService
	projId      string
	envId       string
	attributeId string
	resourceId  *string
}

func (r ApiGetUserAttributeRequest) ResourceId(resourceId string) ApiGetUserAttributeRequest {
	r.resourceId = &resourceId
	return r
}

func (r ApiGetUserAttributeRequest) Execute() (*models.ResourceAttributeRead, *http.Response, error) {
	return r.ApiService.GetUserAttributeExecute(r)
}

/*
GetUserAttribute Get User Attribute

Gets a single attribute defined on the User resource, if such attribute exists.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@param attributeId Either the unique id of the attribute, or the URL-friendly key of the attribute (i.e: the \"slug\").
	@return ApiGetUserAttributeRequest
*/
func (a *UserAttributesApiService) GetUserAttribute(ctx context.Context, projId string, envId string, attributeId string) ApiGetUserAttributeRequest {
	return ApiGetUserAttributeRequest{
		ApiService:  a,
		ctx:         ctx,
		projId:      projId,
		envId:       envId,
		attributeId: attributeId,
	}
}

// Execute executes the request
//
//	@return ResourceAttributeRead
func (a *UserAttributesApiService) GetUserAttributeExecute(r ApiGetUserAttributeRequest) (*models.ResourceAttributeRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodGet
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue *models.ResourceAttributeRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "UserAttributesApiService.GetUserAttribute")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"attribute_id"+"}", url.PathEscape(parameterToString(r.attributeId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.resourceId != nil {
		localVarQueryParams.Add("resource_id", parameterToString(*r.resourceId, ""))
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

type ApiListUserAttributesRequest struct {
	ctx        context.Context
	ApiService *UserAttributesApiService
	projId     string
	envId      string
	resourceId *string
	page       *int32
	perPage    *int32
}

func (r ApiListUserAttributesRequest) ResourceId(resourceId string) ApiListUserAttributesRequest {
	r.resourceId = &resourceId
	return r
}

// Page number of the results to fetch, starting at 1.
func (r ApiListUserAttributesRequest) Page(page int32) ApiListUserAttributesRequest {
	r.page = &page
	return r
}

// The number of results per page (max 100).
func (r ApiListUserAttributesRequest) PerPage(perPage int32) ApiListUserAttributesRequest {
	r.perPage = &perPage
	return r
}

func (r ApiListUserAttributesRequest) Execute() ([]models.ResourceAttributeRead, *http.Response, error) {
	return r.ApiService.ListUserAttributesExecute(r)
}

/*
ListUserAttributes List User Attributes

Lists all the attributes defined on the User resource.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@return ApiListUserAttributesRequest
*/
func (a *UserAttributesApiService) ListUserAttributes(ctx context.Context, projId string, envId string) ApiListUserAttributesRequest {
	return ApiListUserAttributesRequest{
		ApiService: a,
		ctx:        ctx,
		projId:     projId,
		envId:      envId,
	}
}

// Execute executes the request
//
//	@return []ResourceAttributeRead
func (a *UserAttributesApiService) ListUserAttributesExecute(r ApiListUserAttributesRequest) ([]models.ResourceAttributeRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodGet
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue []models.ResourceAttributeRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "UserAttributesApiService.ListUserAttributes")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/users/attributes"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.resourceId != nil {
		localVarQueryParams.Add("resource_id", parameterToString(*r.resourceId, ""))
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

type ApiUpdateUserAttributeRequest struct {
	ctx                     context.Context
	ApiService              *UserAttributesApiService
	projId                  string
	envId                   string
	attributeId             string
	resourceAttributeUpdate *models.ResourceAttributeUpdate
	resourceId              *string
}

func (r ApiUpdateUserAttributeRequest) ResourceAttributeUpdate(resourceAttributeUpdate models.ResourceAttributeUpdate) ApiUpdateUserAttributeRequest {
	r.resourceAttributeUpdate = &resourceAttributeUpdate
	return r
}

func (r ApiUpdateUserAttributeRequest) ResourceId(resourceId string) ApiUpdateUserAttributeRequest {
	r.resourceId = &resourceId
	return r
}

func (r ApiUpdateUserAttributeRequest) Execute() (*models.ResourceAttributeRead, *http.Response, error) {
	return r.ApiService.UpdateUserAttributeExecute(r)
}

/*
UpdateUserAttribute Update User Attribute

Partially updates the attribute defined on the User resource.
Fields that will be provided will be completely overwritten.

	@param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
	@param projId Either the unique id of the project, or the URL-friendly key of the project (i.e: the \"slug\").
	@param envId Either the unique id of the environment, or the URL-friendly key of the environment (i.e: the \"slug\").
	@param attributeId Either the unique id of the attribute, or the URL-friendly key of the attribute (i.e: the \"slug\").
	@return ApiUpdateUserAttributeRequest
*/
func (a *UserAttributesApiService) UpdateUserAttribute(ctx context.Context, projId string, envId string, attributeId string) ApiUpdateUserAttributeRequest {
	return ApiUpdateUserAttributeRequest{
		ApiService:  a,
		ctx:         ctx,
		projId:      projId,
		envId:       envId,
		attributeId: attributeId,
	}
}

// Execute executes the request
//
//	@return ResourceAttributeRead
func (a *UserAttributesApiService) UpdateUserAttributeExecute(r ApiUpdateUserAttributeRequest) (*models.ResourceAttributeRead, *http.Response, error) {
	var (
		localVarHTTPMethod  = http.MethodPatch
		localVarPostBody    interface{}
		formFiles           []formFile
		localVarReturnValue *models.ResourceAttributeRead
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "UserAttributesApiService.UpdateUserAttribute")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"proj_id"+"}", url.PathEscape(parameterToString(r.projId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"env_id"+"}", url.PathEscape(parameterToString(r.envId, "")), -1)
	localVarPath = strings.Replace(localVarPath, "{"+"attribute_id"+"}", url.PathEscape(parameterToString(r.attributeId, "")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}
	if r.resourceAttributeUpdate == nil {
		return localVarReturnValue, nil, reportError("resourceAttributeUpdate is required and must be specified")
	}

	if r.resourceId != nil {
		localVarQueryParams.Add("resource_id", parameterToString(*r.resourceId, ""))
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
	localVarPostBody = r.resourceAttributeUpdate
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
