# Content Storage Server - Management Interface

A comprehensive web-based management dashboard for the Content Storage Server built with Bootstrap CSS and modular JavaScript. Features a fixed-height, no-scroll design that fits entirely within the browser viewport.

## Features

### 🎛️ **Server Health Monitoring**
- Real-time server status with adaptive status indicators (Online/Degraded/Offline)
- Server uptime tracking since startup
- Content count monitoring with storage connectivity verification
- Component health status for database, backup, and GC systems
- Last updated timestamp with automatic refresh

### 📊 **System Metrics Dashboard**
- **Database Metrics**: Size monitoring, health status tracking
- **Backup Statistics**: Count tracking, last backup timestamp
- **Garbage Collection**: Run count, last execution time, performance stats
- **Queue Metrics**: Write queue depth, processing rate, error tracking
- **Access Manager Stats**: Tracked content count, cleanup intervals, memory usage

### 🔧 **Management Operations**
- **Trigger Sync**: Manual queue flush and database synchronization
- **Create Backup**: Generate immediate database backup with monitoring
- **Run GC**: Manual garbage collection with performance tracking
- **Cleanup Trackers**: Remove stale access tracking data to prevent memory leaks

### 📋 **Operation Logging**
- Real-time visual log of all management operations
- Success/error status tracking with detailed information
- Operation duration monitoring and endpoint tracking
- Automatic log rotation (last 10 entries)
- Clear logs functionality

### 💾 **Backup Management**
- Display of available backup files with timestamps
- Backup count and last backup time tracking
- Recent backup highlighting
- Backup file details (placeholder for future expansion)

### 📁 **Content Management**
- **Manual Content Loading**: Content is loaded on-demand, not automatically
- Pagination support with configurable items per page
- Advanced search functionality with visual feedback
- Content deletion with confirmation dialogs
- Access count tracking and expiration monitoring
- Content type and tag display

### 🔄 **Smart Updates**
- Auto-refresh for health and metrics (every 30 seconds)
- Manual refresh capability for all sections
- Content management requires manual loading for performance
- Loading states and comprehensive error handling

## File Structure

```
static/
├── index.html          # Main management interface
├── js/
│   └── management.js   # Modular JavaScript functions
├── css/
│   └── custom.css      # Additional styling
└── README.md          # This file
```

## Architecture

### **Layout Design**
- **Fixed-Height Layout**: No-scroll design that fits entirely within viewport
- **Four Main Sections**: Dashboard (17vh), Advanced Metrics (15vh), Logs/Backups (30vh), Content Management (34vh)
- **Bootstrap 5.3.0** for responsive design and components
- **Bootstrap Icons** for consistent iconography
- **Mobile-Responsive**: Adaptive heights and spacing for different screen sizes

### **JavaScript Modules**
The `management.js` file is organized into modular functions:

#### **Core Functions**
- `initializeManagement()` - Initialize interface with health/metrics/backups (not content)
- `refreshAll()` - Refresh health, metrics, and backups only
- `setupAutoRefresh()` - Auto-refresh health and metrics every 30 seconds
- `setupEventListeners()` - Configure search and interaction handlers

#### **API Communication**
- `apiRequest()` - Centralized API request handling with error management
- `getRequestHeaders()` - Authentication header management
- `checkAuthentication()` - API key validation with user prompting

#### **Data Loading Functions**
- `loadHealthData()` - Server health with component status and uptime
- `loadMetrics()` - System metrics, queue metrics, and access manager stats
- `loadContent()` - Manual content loading with pagination and search
- `loadBackups()` - Available backup files with metadata

#### **Management Operations**
- `triggerSync()` - Manual queue flush and database synchronization
- `createBackup()` - Backup creation with immediate flag
- `runGarbageCollection()` - Manual GC trigger with performance tracking
- `cleanupAccessTrackers()` - Remove stale access tracking data
- `deleteContent()` - Content deletion with confirmation

#### **Operation Logging**
- `addOperationLog()` - Add visual log entries with status and details
- `clearOperationLogs()` - Clear operation log display

#### **Search & Content Management**
- `performSearch()` - Search content with visual feedback
- `clearSearch()` - Clear search and return to empty state
- `updateSearchVisualFeedback()` - Visual search state indicators

#### **UI Utilities**
- `showAlert()` - Navbar alert notifications with auto-dismiss
- `displayContent()` - Content table rendering with access counts
- `displayMetrics()` - System metrics display with health indicators
- `displayQueueMetrics()` - Queue health and performance metrics
- `displayAccessManagerStats()` - Access manager statistics
- `displayBackups()` - Backup file listing with timestamps
- `updatePagination()` - Pagination controls for content
- `formatBytes()`, `formatDate()`, `formatDuration()` - Data formatting utilities

### **CSS Styling**
- **Fixed-Height Layout**: Prevents page scrolling, fits in viewport
- **Flexbox Layout**: Responsive card arrangement with proper overflow handling
- **Scrollable Sections**: Operation logs, backup lists, and content table
- **Visual Feedback**: Loading states, hover effects, and status indicators
- **Mobile Optimization**: Adaptive heights and compact spacing

## Usage

### **Access the Interface**
1. Start the Content Storage Server
2. Navigate to `http://localhost:8081/` (or your configured host/port)
3. The management interface loads with health, metrics, and backup data automatically
4. Content management requires manual loading for performance

### **Authentication**
- If server authentication is enabled (`ENABLE_AUTH=true`), you'll be prompted for an API key
- The API key can be provided via:
  - Browser prompt (automatic when accessing protected endpoints)
  - `X-API-Key` header
  - `api_key` query parameter

### **Dashboard Sections**

#### **Server Health (Top Left)**
- **Overall Status**: Healthy/Degraded/Unhealthy with color-coded indicators
- **Uptime**: Server runtime since startup
- **Content Count**: Total stored content items
- **Component Health**: Database, backup, and GC system status (when available)

#### **System Metrics (Top Center)**
- **Database Size**: Storage usage in human-readable format
- **Health Status**: Overall system health indicator
- **Backup Count**: Number of available backups
- **Last Backup**: Timestamp of most recent backup
- **GC Statistics**: Garbage collection runs and last execution

#### **Management Operations (Top Right)**
- **Trigger Sync**: Manual queue flush and database synchronization
- **Create Backup**: Generate immediate backup with monitoring
- **Run GC**: Manual garbage collection with performance tracking
- **Cleanup Trackers**: Remove stale access tracking data

#### **Advanced Metrics (Second Row)**
- **Queue Metrics**: Write queue depth, processing rate, error tracking
- **Access Manager Stats**: Tracked content, total accesses, cleanup intervals

#### **Operation Logs (Bottom Left)**
- Real-time log of management operations with success/error status
- Duration tracking and detailed response information
- Automatic rotation (last 10 entries) with clear functionality

#### **Available Backups (Bottom Right)**
- List of backup files with timestamps and recent indicators
- Backup count summary and last backup time
- Backup details (placeholder for future expansion)

#### **Content Management (Bottom Full Width)**
- **Manual Loading**: Click "Load Content" to fetch data (not automatic)
- **Search**: Real-time search with visual feedback and debouncing
- **Pagination**: Navigate through content with configurable page size
- **Actions**: Delete content with confirmation, view content (placeholder)
- **Metadata**: Content type, tags, creation/expiration dates, access counts

## API Endpoints

The management interface interacts with the following server endpoints:

### **Health & Monitoring**
- `GET /health` - Basic health check with storage connectivity
- `GET /health/detailed` - Comprehensive health with adaptive status codes (200/206/503)
- `GET /api/v1/metrics` - System metrics including queue performance and health status

### **Management Operations**
- `POST /api/v1/sync` - Manual queue flush and database synchronization
- `POST /api/v1/backup` - Create manual backup with comprehensive monitoring
- `POST /api/v1/gc` - Trigger garbage collection with performance tracking
- `POST /api/v1/cleanup` - Cleanup access trackers to prevent memory leaks

### **Content Management**
- `GET /api/v1/content/` - List content with pagination and filtering
- `GET /api/v1/content/count` - Get total content count
- `GET /api/v1/content/{id}` - Retrieve specific content by ID
- `DELETE /api/v1/content/{id}` - Delete content by ID
- `GET /api/v1/content/{id}/status` - Check content storage status

## Configuration

The interface automatically adapts to server configuration:

- **Authentication**: Prompts for API key when server returns 401 status
- **Pagination**: Configurable items per page (default: 10, configurable in CONFIG.itemsPerPage)
- **Auto-refresh**: 30-second interval for health and metrics only (configurable in CONFIG.refreshInterval)
- **Content Loading**: Manual only - content is not loaded automatically for performance
- **Error Handling**: Graceful degradation with user-friendly error messages
- **Search Debouncing**: 500ms delay for search-as-you-type functionality

## Browser Compatibility

- **Modern Browsers**: Chrome 90+, Firefox 88+, Safari 14+, Edge 90+
- **Mobile**: iOS Safari 14+, Chrome Mobile 90+
- **Features**: ES6+ JavaScript, CSS Grid, Flexbox

## Security Considerations

- **XSS Prevention**: HTML escaping for user content
- **CSRF Protection**: API key authentication
- **Content Security**: Bootstrap CDN with integrity checks
- **Input Validation**: Client-side validation with server-side enforcement

## Current Implementation Status

### **✅ Fully Implemented**
- Fixed-height, no-scroll dashboard layout
- Real-time health monitoring with component status
- System metrics with queue and access manager stats
- Management operations (sync, backup, GC, cleanup) with logging
- Operation logging with visual feedback and status tracking
- Backup file listing and management
- Manual content loading with pagination
- Search functionality with visual feedback and debouncing
- Authentication support with API key prompting
- Mobile-responsive design with adaptive layouts

### **🔄 Partially Implemented**
- Content viewing (placeholder - shows alert message)
- Backup details (placeholder - shows info message)
- Advanced filtering (basic search implemented)

### **📋 Future Enhancements**
- [ ] Content viewing modal with syntax highlighting
- [ ] Backup restoration functionality
- [ ] Real-time WebSocket updates for live metrics
- [ ] Configuration management interface
- [ ] Server log viewing and analysis
- [ ] Performance charts and graphs with historical data
- [ ] Advanced content filtering (by type, tag, date range)
- [ ] User management (multi-user support)
- [ ] Export functionality for metrics and logs
- [ ] Dark/light theme toggle
- [ ] Keyboard shortcuts for common operations

## Development

### **Adding New Features**
1. **HTML Structure**: Add to appropriate section in `index.html` (dashboard, advanced-metrics, logs-backups, or content-management blocks)
2. **JavaScript Functions**: Implement in `management.js` following the modular pattern
3. **CSS Styling**: Add to `custom.css` respecting the fixed-height layout constraints
4. **API Integration**: Use the `apiRequest()` function for server communication
5. **Update Documentation**: Update this README with new functionality

### **Layout Constraints**
- **Fixed Heights**: Dashboard (17vh), Advanced Metrics (15vh), Logs/Backups (30vh), Content Management (34vh)
- **No Page Scrolling**: All content must fit within viewport
- **Responsive Design**: Test on mobile devices with adaptive heights
- **Overflow Handling**: Use scrollable containers for dynamic content

### **Testing Checklist**
- [ ] Test with authentication enabled/disabled
- [ ] Verify responsive design on mobile devices (768px, 576px breakpoints)
- [ ] Test error handling with server offline
- [ ] Validate all management operations (sync, backup, GC, cleanup)
- [ ] Test content loading, search, and pagination
- [ ] Verify operation logging functionality
- [ ] Test auto-refresh behavior (health/metrics only)
- [ ] Validate accessibility with screen readers
- [ ] Test API key authentication flow

## Technical Details

### **Performance Considerations**
- **Manual Content Loading**: Content is not loaded automatically to prevent performance issues with large datasets
- **Auto-refresh Strategy**: Only health and metrics are auto-refreshed (30s interval), content requires manual refresh
- **Search Debouncing**: 500ms delay prevents excessive API calls during typing
- **Pagination**: Configurable page size (default: 10) for efficient content browsing
- **Fixed Layout**: No-scroll design prevents layout shifts and improves user experience

### **Error Handling**
- **Graceful Degradation**: Interface continues to function even if some endpoints fail
- **User Feedback**: Clear error messages in navbar alerts with auto-dismiss
- **Retry Mechanisms**: Failed operations can be retried manually
- **Loading States**: Visual indicators for all async operations

### **Security Features**
- **XSS Prevention**: All user content is properly escaped before display
- **API Key Authentication**: Secure authentication with automatic prompting
- **Input Validation**: Client-side validation with server-side enforcement
- **CORS Support**: Proper cross-origin resource sharing configuration

## Dependencies

### **External CDN Resources**
- Bootstrap 5.3.0 CSS and JS with integrity checks
- Bootstrap Icons 1.10.0 for consistent iconography
- No additional JavaScript libraries required (vanilla JS implementation)

### **Server Dependencies**
- Content Storage Server with Echo framework
- Static file serving enabled for management interface
- CORS configured for web interface access
- Authentication middleware (optional, based on ENABLE_AUTH configuration)
