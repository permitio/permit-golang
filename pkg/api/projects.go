package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type Projects struct {
	permitBaseApi
}

func NewProjectsApi(client *openapi.APIClient, config *config.PermitConfig) *Projects {
	return &Projects{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List all projects in the organization, requires Project level API key, or higher.
// Usage Example:
// `projects, err := PermitClient.Api.Projects.List(ctx, 1, 10)`
func (p *Projects) List(ctx context.Context, page int, perPage int) ([]models.ProjectRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		p.logger.Error("error listing projects - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := p.lazyLoadPermitContext(ctx, config.ProjectAPIKeyLevel)
	if err != nil {
		p.logger.Error("", zap.Error(err))
		return nil, err
	}
	projects, httpRes, err := p.client.ProjectsApi.ListProjects(ctx).Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error listing projects", zap.Error(err))
		return nil, err
	}

	return projects, nil
}

// Get a project by key, requires Project level API key, or higher.
// Usage Example:
// `project, err := PermitClient.Api.Projects.Get(ctx, "project-key")`
func (p *Projects) Get(ctx context.Context, projectKey string) (*models.ProjectRead, error) {
	err := p.lazyLoadPermitContext(ctx, config.ProjectAPIKeyLevel)
	if err != nil {
		p.logger.Error("", zap.Error(err))
		return nil, err
	}
	project, httpRes, err := p.client.ProjectsApi.GetProject(ctx, projectKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error getting project: "+projectKey, zap.Error(err))
		return nil, err
	}
	return project, nil
}

// GetByKey get a project by key, requires Project level API key, or higher.
// Usage Example:
// `project, err := PermitClient.Api.Projects.GetByKey(ctx, "project-key")`
func (p *Projects) GetByKey(ctx context.Context, projectKey string) (*models.ProjectRead, error) {
	return p.Get(ctx, projectKey)
}

// GetById get a project by id, requires Project level API key, or higher.
// Usage Example:
// `project, err := PermitClient.Api.Projects.GetById(ctx, uuid.New())`
func (p *Projects) GetById(ctx context.Context, projectId uuid.UUID) (*models.ProjectRead, error) {
	return p.Get(ctx, projectId.String())
}

// Create a new project, requires Project level API key, or higher.
// Usage Example:
// ```
// projectCreate := models.NewProjectCreate("project-key", "project-name")
// project, err := PermitClient.Api.Projects.Create(ctx, projectCreate)
// ```
func (p *Projects) Create(ctx context.Context, projectCreate models.ProjectCreate) (*models.ProjectRead, error) {
	err := p.lazyLoadPermitContext(ctx, config.ProjectAPIKeyLevel)
	if err != nil {
		p.logger.Error("", zap.Error(err))
		return nil, err
	}
	project, httpRes, err := p.client.ProjectsApi.CreateProject(ctx).ProjectCreate(projectCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error creating project: "+projectCreate.GetKey(), zap.Error(err))
		return nil, err
	}
	return project, nil
}

// Update a project, requires Project level API key, or higher.
// Usage Example:
// ```
// projectUpdate := models.NewProjectUpdate()
// projectUpdate.SetName("new-project-name")
// project, err := PermitClient.Api.Projects.Update(ctx, "project-key", projectUpdate)
// ```
func (p *Projects) Update(ctx context.Context, projectKey string, projectUpdate models.ProjectUpdate) (*models.ProjectRead, error) {
	err := p.lazyLoadPermitContext(ctx, config.ProjectAPIKeyLevel)
	if err != nil {
		p.logger.Error("", zap.Error(err))
		return nil, err
	}
	project, httpRes, err := p.client.ProjectsApi.UpdateProject(ctx, projectKey).ProjectUpdate(projectUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error updating project: "+projectKey, zap.Error(err))
		return nil, err
	}
	return project, nil
}

// Delete a project, requires Project level API key, or higher.
// Usage Example:
// `err := PermitClient.Api.Projects.Delete(ctx, "project-key")`
func (p *Projects) Delete(ctx context.Context, projectKey string) error {
	err := p.lazyLoadPermitContext(ctx, config.ProjectAPIKeyLevel)
	if err != nil {
		p.logger.Error("", zap.Error(err))
		return err
	}
	httpRes, err := p.client.ProjectsApi.DeleteProject(ctx, projectKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error deleting project: "+projectKey, zap.Error(err))
		return err
	}
	return nil
}
