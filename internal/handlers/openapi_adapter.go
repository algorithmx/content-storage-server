package handlers

import (
	"content-storage-server/api"
	"content-storage-server/pkg/config"
	"content-storage-server/pkg/storage"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// OpenAPIAdapter implements the generated ServerInterface by wrapping existing handlers
type OpenAPIAdapter struct {
	contentHandler *ContentHandler
	healthHandler  *HealthHandler
}

// NewOpenAPIAdapter creates a new adapter
func NewOpenAPIAdapter(store storage.Storage, logger *zap.Logger, cfg *config.Config) *OpenAPIAdapter {
	return &OpenAPIAdapter{
		contentHandler: NewContentHandler(store, logger, cfg),
		healthHandler:  NewHealthHandler(store, logger),
	}
}

// Content Operations

func (a *OpenAPIAdapter) StoreContent(ctx echo.Context) error {
	return a.contentHandler.StoreContent(ctx)
}

func (a *OpenAPIAdapter) ListContent(ctx echo.Context, params api.ListContentParams) error {
	return a.contentHandler.ListContent(ctx)
}

func (a *OpenAPIAdapter) GetContent(ctx echo.Context, id string) error {
	return a.contentHandler.GetContent(ctx)
}

func (a *OpenAPIAdapter) DeleteContent(ctx echo.Context, id string) error {
	return a.contentHandler.DeleteContent(ctx)
}

func (a *OpenAPIAdapter) GetContentStatus(ctx echo.Context, id string) error {
	return a.contentHandler.GetContentStatus(ctx)
}

func (a *OpenAPIAdapter) GetContentCount(ctx echo.Context) error {
	return a.contentHandler.GetContentCount(ctx)
}

// Management Operations

func (a *OpenAPIAdapter) TriggerSync(ctx echo.Context) error {
	return a.healthHandler.TriggerSync(ctx)
}

func (a *OpenAPIAdapter) CreateBackup(ctx echo.Context) error {
	return a.healthHandler.CreateBackup(ctx)
}

func (a *OpenAPIAdapter) TriggerGC(ctx echo.Context) error {
	return a.healthHandler.TriggerGC(ctx)
}

func (a *OpenAPIAdapter) CleanupAccessTrackers(ctx echo.Context) error {
	return a.healthHandler.CleanupAccessTrackers(ctx)
}

// Health & Monitoring

func (a *OpenAPIAdapter) GetMetrics(ctx echo.Context) error {
	return a.healthHandler.GetMetrics(ctx)
}

func (a *OpenAPIAdapter) HealthCheck(ctx echo.Context) error {
	return a.healthHandler.HealthCheck(ctx)
}

func (a *OpenAPIAdapter) DetailedHealthCheck(ctx echo.Context) error {
	return a.healthHandler.DetailedHealthCheck(ctx)
}
