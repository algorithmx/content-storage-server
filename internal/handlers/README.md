# Handlers Module

HTTP request handlers for the Content Storage Server, providing RESTful API endpoints for content management, health monitoring, and system operations.

## Overview

The handlers module implements the HTTP layer of the Content Storage Server, providing a comprehensive RESTful API for content operations, system management, and health monitoring. Built on the Echo framework, it features enterprise-grade middleware integration, comprehensive validation, and robust error handling.

## Key Features

- **RESTful API Design**: Clean, intuitive endpoints following REST principles
- **Comprehensive Validation**: Multi-layer request validation with performance optimization
- **Asynchronous Processing**: Non-blocking content storage with queue-based processing
- **Health Monitoring**: Basic and detailed health checks with metrics
- **System Management**: Backup, sync, and garbage collection operations
- **Security Integration**: API key authentication and IP allowlisting
- **Swagger Documentation**: Auto-generated API documentation
- **Error Handling**: Consistent error responses with detailed information
- **Performance Optimization**: Efficient pagination, filtering, and caching

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Echo HTTP Framework                      │
├─────────────────────────────────────────────────────────────┤
│                   Middleware Stack                          │
│  Shutdown → Rate Limiting → Security → Auth → Compression  │
├─────────────────┬─────────────────┬─────────────────────────┤
│  ContentHandler │  HealthHandler  │   ShutdownHandler       │
├─────────────────┼─────────────────┼─────────────────────────┤
│ Request         │ Swagger         │   Validation            │
│ Validation      │ Models          │   Logic                 │
├─────────────────┴─────────────────┴─────────────────────────┤
│                   Storage Layer                             │
└─────────────────────────────────────────────────────────────┘
```

## Handler Components

### ContentHandler
Manages all content-related operations:
- **CRUD Operations**: Create, read, update, delete content
- **Pagination**: Efficient listing with offset/limit support
- **Filtering**: Advanced content filtering by type, tag, dates
- **Access Tracking**: Automatic access count management
- **Status Checking**: Queue status and storage completion tracking

### HealthHandler
Provides system monitoring and management:
- **Health Checks**: Basic and detailed system health reporting
- **Metrics**: Performance and operational metrics
- **System Operations**: Manual backup, sync, and GC triggers
- **Access Cleanup**: Memory management for access tracking

### ShutdownHandler
Manages graceful server shutdown:
- **Shutdown Coordination**: Atomic shutdown state management
- **Request Rejection**: Immediate 503 responses during shutdown
- **Middleware Integration**: First-line defense against new requests

### RequestValidator
Comprehensive request validation:
- **Performance Mode**: Optimized validation for high-throughput
- **Security Validation**: Input sanitization and security checks
- **Content Validation**: Type-specific content validation
- **Pagination Validation**: Parameter validation with limits

## API Endpoints

### Content Management

#### Store Content
- **Endpoint**: `POST /api/v1/content/`
- **Description**: Store new content with asynchronous processing
- **Authentication**: Required (if enabled)
- **Request Body**: JSON with id, data, type, tag, expires_at, access_limit
- **Response**: HTTP 202 (Accepted) - Content queued for storage
- **Features**: Sequential write processing, emergency shutdown preservation

#### Get Content
- **Endpoint**: `GET /api/v1/content/{id}`
- **Description**: Retrieve content by ID with access tracking
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 with content data and access count
- **Features**: Atomic access increment, expiration checking

#### List Content
- **Endpoint**: `GET /api/v1/content/`
- **Description**: List content with pagination and filtering
- **Authentication**: Required (if enabled)
- **Query Parameters**: limit, offset, type, tag, created_after, created_before, include_expired
- **Response**: HTTP 200 with content array and pagination metadata
- **Features**: Advanced filtering, access count inclusion

#### Delete Content
- **Endpoint**: `DELETE /api/v1/content/{id}`
- **Description**: Delete content with write conflict handling
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 on success, HTTP 404 if not found
- **Features**: Retry logic for write conflicts, access tracking cleanup

#### Get Content Count
- **Endpoint**: `GET /api/v1/content/count`
- **Description**: Get total content count
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 with count value
- **Features**: Storage-level optimization, no iteration required

#### Get Content Status
- **Endpoint**: `GET /api/v1/content/{id}/status`
- **Description**: Check content storage status
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 with status (queued, stored, not_found)
- **Features**: Queue visibility, processing flow tracking

### Health & Monitoring

#### Basic Health Check
- **Endpoint**: `GET /health`
- **Description**: Basic system health check
- **Authentication**: Not required
- **Response**: HTTP 200 with status and basic metrics
- **Features**: Fast response, minimal overhead

#### Detailed Health Check
- **Endpoint**: `GET /health/detailed`
- **Description**: Comprehensive health check with detailed metrics
- **Authentication**: Not required
- **Response**: HTTP 200/206/503 based on health status
- **Features**: Component health, queue metrics, performance data

#### Get Metrics
- **Endpoint**: `GET /api/v1/metrics`
- **Description**: System performance and operational metrics
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 with comprehensive metrics
- **Features**: Storage stats, queue metrics, GC information

### System Management

#### Trigger Sync
- **Endpoint**: `POST /api/v1/sync`
- **Description**: Manual queue flush operation
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 on success
- **Features**: Force queue processing, operational control

#### Create Backup
- **Endpoint**: `POST /api/v1/backup`
- **Description**: Manual backup creation
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 on success
- **Features**: Immediate backup, integrity validation

#### Cleanup Access Trackers
- **Endpoint**: `POST /api/v1/cleanup`
- **Description**: Clean up access tracking memory
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 with cleanup statistics
- **Features**: Memory optimization, leak prevention

#### Trigger Garbage Collection
- **Endpoint**: `POST /api/v1/gc`
- **Description**: Manual garbage collection
- **Authentication**: Required (if enabled)
- **Response**: HTTP 200 with GC statistics
- **Features**: Space reclamation, performance optimization

## Request/Response Models

### StorageRequest
```go
type StorageRequest struct {
    ID          string     `json:"id" validate:"required,max=255"`
    Data        string     `json:"data" validate:"required"`
    Type        string     `json:"type" validate:"required,max=100"`
    Tag         string     `json:"tag,omitempty" validate:"omitempty,max=100"`
    ExpiresAt   *time.Time `json:"expires_at,omitempty"`
    AccessLimit int        `json:"access_limit,omitempty" validate:"omitempty,min=0,max=1000000"`
}
```

### StorageResponse
```go
type StorageResponse struct {
    Success bool        `json:"success"`
    Message string      `json:"message,omitempty"`
    Data    interface{} `json:"data,omitempty"`
}
```

### ContentFilter
```go
type ContentFilter struct {
    ContentType    string     // MIME type filter
    Tag           string     // Tag filter
    CreatedAfter  *time.Time // Created after timestamp
    CreatedBefore *time.Time // Created before timestamp
    IncludeExpired bool       // Include expired content
}
```

## Validation Rules

### ID Validation
- Required, non-empty
- Maximum 255 characters
- Alphanumeric with dots, hyphens, underscores only
- No dangerous path patterns (.., leading/trailing dots)
- Valid UTF-8 encoding

### Content Type Validation
- Required MIME type format (type/subtype)
- Must be in ALLOWED_CONTENT_TYPES configuration
- Maximum 100 characters

### Data Validation
- Required, non-empty
- Size limited by MAX_CONTENT_SIZE configuration (default: 10MB)
- Type-specific validation based on content type

### Tag Validation
- Optional field
- Maximum 100 characters
- Alphanumeric with dots, hyphens, underscores only
- Valid UTF-8 encoding

### Pagination Validation
- Limit: 1-1000 (default: 100)
- Offset: >= 0 (default: 0)
- Hardcoded maximum for performance protection

## Middleware Integration

### Middleware Stack Order
1. **Shutdown Middleware** - Immediately refuses requests during shutdown
2. **Request ID** - Adds unique request tracking
3. **Rate Limiting** - Echo and custom rate limiters
4. **IP Allowlisting** - Network-level security
5. **Panic Recovery** - Graceful error handling
6. **Security Headers** - CORS and security policies
7. **Compression** - Response compression (if enabled)
8. **Request Timeout** - Configurable request timeouts
9. **Authentication** - API key validation (if enabled)

### Authentication
- **API Key Methods**: Header (`X-API-Key`) or Query Parameter (`api_key`)
- **Protected Endpoints**: All `/api/v1/*` routes (when `ENABLE_AUTH=true`)
- **Public Endpoints**: Health checks, static files, documentation
- **Bypass Logic**: Automatic bypass for health and static routes

### Rate Limiting
- **Echo Rate Limiter**: Built-in first-line defense
- **Custom Rate Limiter**: Advanced throttling with backlog support
- **Backlog Handling**: Queues requests during traffic spikes
- **Response**: HTTP 429 when limits exceeded

## Error Handling

### Error Response Format
```json
{
    "success": false,
    "message": "Human-readable error message",
    "data": {
        "error": "Detailed error information",
        "code": "ERROR_CODE",
        "timestamp": "2024-01-01T00:00:00Z"
    }
}
```

### HTTP Status Codes
- **200 OK**: Successful operation
- **202 Accepted**: Content queued for processing
- **206 Partial Content**: System degraded but functional
- **400 Bad Request**: Validation errors, invalid input
- **401 Unauthorized**: Authentication required
- **404 Not Found**: Content not found
- **410 Gone**: Content expired or access limit reached
- **413 Payload Too Large**: Content size exceeds limit
- **415 Unsupported Media Type**: Invalid content type
- **429 Too Many Requests**: Rate limit exceeded
- **500 Internal Server Error**: Server-side errors
- **503 Service Unavailable**: Server shutting down or unhealthy

### Error Categories
- **Validation Errors**: Input validation failures
- **Authentication Errors**: API key issues
- **Storage Errors**: Database operation failures
- **Rate Limiting Errors**: Throttling violations
- **System Errors**: Internal server problems

## Configuration Dependencies

### Required Environment Variables
- **HOST**: Server bind address (default: "0.0.0.0")
- **PORT**: Server port (default: "8080")
- **API_KEY**: Authentication key (when ENABLE_AUTH=true)

### Content Validation Settings
- **MAX_CONTENT_SIZE**: Maximum content size in bytes (default: 10MB)
- **ALLOWED_CONTENT_TYPES**: Comma-separated MIME types (default: "application/json,text/plain")
- **MAX_ID_LENGTH**: Maximum ID length (default: 255)

### Performance Settings
- **PERFORMANCE_MODE**: Enable optimized validation (default: true)
- **ENABLE_COMPRESSION**: Enable response compression (default: true)
- **COMPRESSION_LEVEL**: Gzip compression level (default: 6)

### Security Settings
- **ENABLE_AUTH**: Enable API key authentication (default: false)
- **ALLOWED_IPS**: Comma-separated IP allowlist
- **ALLOWED_ORIGINS**: CORS allowed origins

### Rate Limiting Settings
- **THROTTLE_LIMIT**: Requests per second limit
- **THROTTLE_BACKLOG_LIMIT**: Backlog queue size
- **THROTTLE_BACKLOG_TIMEOUT**: Backlog timeout duration
- **ECHO_RATE_LIMIT**: Echo rate limiter requests per second

## Usage Examples

### Store Content
```bash
curl -X POST http://localhost:8080/api/v1/content/ \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "id": "user-123-document",
    "data": "Hello, World!",
    "type": "text/plain",
    "tag": "user-documents",
    "access_limit": 10
  }'
```

### Get Content
```bash
curl -X GET http://localhost:8080/api/v1/content/user-123-document \
  -H "X-API-Key: your-api-key"
```

### List Content with Filtering
```bash
curl -X GET "http://localhost:8080/api/v1/content/?limit=50&offset=0&type=text/plain&tag=user-documents" \
  -H "X-API-Key: your-api-key"
```

### Check Content Status
```bash
curl -X GET http://localhost:8080/api/v1/content/user-123-document/status \
  -H "X-API-Key: your-api-key"
```

### Health Check
```bash
curl -X GET http://localhost:8080/health
```

### Detailed Health Check
```bash
curl -X GET http://localhost:8080/health/detailed
```

### Trigger Manual Backup
```bash
curl -X POST http://localhost:8080/api/v1/backup \
  -H "X-API-Key: your-api-key"
```

## Performance Considerations

### Validation Optimization
- **Performance Mode**: Enables fast-path validation for high throughput
- **Struct Validation**: Uses go-playground/validator for efficient validation
- **Early Validation**: Size and basic checks before expensive operations

### Asynchronous Processing
- **Non-blocking Writes**: Content storage returns immediately after queuing
- **Sequential Processing**: Ensures data consistency and ordering
- **Status Tracking**: Allows clients to check processing completion

### Pagination Efficiency
- **Stream-based Iteration**: Minimizes memory usage for large datasets
- **Filter-aware Counting**: Optimized counting for filtered results
- **Hardcoded Limits**: Prevents resource exhaustion

### Caching and Compression
- **Response Compression**: Automatic gzip compression for bandwidth optimization
- **Static File Caching**: Efficient serving of management interface assets
- **Access Count Caching**: In-memory access tracking for performance

## File Structure

```
internal/handlers/
├── README.md              # This documentation
├── content.go            # Content management handlers
├── health.go             # Health monitoring and system management
├── shutdown.go           # Graceful shutdown coordination
├── validation.go         # Request validation logic
└── swagger_models.go     # Swagger documentation models
```

### File Responsibilities

#### content.go
- Content CRUD operations
- Pagination and filtering logic
- Access tracking integration
- Status checking functionality

#### health.go
- Basic and detailed health checks
- System metrics collection
- Manual operation triggers (backup, sync, GC)
- Access tracker cleanup

#### shutdown.go
- Shutdown state management
- Request rejection during shutdown
- Middleware integration
- Atomic state operations

#### validation.go
- Request validation logic
- Performance optimization
- Security validation
- Content filter parsing

#### swagger_models.go
- Swagger documentation models
- Request/response type definitions
- API documentation annotations
- Example data structures

## Best Practices

### Handler Implementation
- Use structured logging with request context
- Implement proper error handling and recovery
- Validate all inputs before processing
- Return consistent response formats

### Performance Optimization
- Enable performance mode for high-throughput scenarios
- Use appropriate pagination limits
- Implement efficient filtering logic
- Monitor and optimize slow endpoints

### Security
- Always validate and sanitize inputs
- Use proper authentication for protected endpoints
- Implement rate limiting for public endpoints
- Log security-relevant events

### Error Handling
- Provide clear, actionable error messages
- Use appropriate HTTP status codes
- Log errors with sufficient context
- Implement retry logic where appropriate

## Troubleshooting

### Common Issues

1. **Authentication Failures**: Check API key configuration and headers
2. **Validation Errors**: Review input format and size limits
3. **Rate Limiting**: Monitor request rates and adjust limits
4. **Performance Issues**: Enable performance mode and check pagination
5. **Storage Errors**: Check storage health and queue status

### Debug Information

Use the health and metrics endpoints to gather debug information:

```bash
# Check system health
curl http://localhost:8080/health/detailed

# Get system metrics
curl -H "X-API-Key: your-key" http://localhost:8080/api/v1/metrics

# Check content status
curl -H "X-API-Key: your-key" http://localhost:8080/api/v1/content/{id}/status
```

## Contributing

When contributing to the handlers module:

1. Follow RESTful API design principles
2. Implement comprehensive input validation
3. Add appropriate Swagger documentation
4. Include error handling for all scenarios
5. Write tests for all endpoints
6. Update this documentation for any changes

## License

This module is part of the content storage server project and follows the same licensing terms.
