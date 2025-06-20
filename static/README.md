# Content Storage Server - Management Interface

A minimalist web-based management interface for the Content Storage Server built with Bootstrap CSS and modular JavaScript.

## Features

### 🎛️ **Server Health Monitoring**
- Real-time server status display
- Uptime tracking
- Content count monitoring
- Last updated timestamp

### 📊 **System Metrics**
- Database size monitoring
- Backup count tracking
- Conflict resolution statistics
- Last backup timestamp

### 🔧 **Management Operations**
- **Trigger Sync**: Manually initiate synchronization
- **Create Backup**: Generate immediate database backup
- **Run GC**: Manually trigger garbage collection
- **Cleanup Trackers**: Remove stale access tracking data to prevent memory leaks

### 📁 **Content Management**
- View all stored content with pagination
- Search functionality (when implemented server-side)
- Delete content items with confirmation
- View content details (placeholder for future implementation)
- Access count and expiration tracking

### 🔄 **Real-time Updates**
- Auto-refresh every 30 seconds
- Manual refresh capability
- Loading states and error handling

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

### **HTML Structure**
- **Bootstrap 5.3.0** for responsive design and components
- **Bootstrap Icons** for consistent iconography
- Semantic HTML structure with accessibility considerations
- Mobile-responsive layout

### **JavaScript Modules**
The `management.js` file is organized into modular functions:

#### **Core Functions**
- `initializeManagement()` - Initialize the interface
- `refreshAll()` - Refresh all data sections
- `setupAutoRefresh()` - Configure automatic updates

#### **API Communication**
- `apiRequest()` - Centralized API request handling
- `getRequestHeaders()` - Authentication header management
- `checkAuthentication()` - API key validation

#### **Data Loading**
- `loadHealthData()` - Server health information
- `loadMetrics()` - System metrics
- `loadContent()` - Content listing with pagination

#### **Management Operations**
- `triggerSync()` - Manual synchronization
- `createBackup()` - Backup creation
- `deleteContent()` - Content deletion

#### **UI Utilities**
- `showAlert()` - User notifications
- `displayContent()` - Content table rendering
- `updatePagination()` - Pagination controls
- `formatBytes()`, `formatDate()`, `formatDuration()` - Data formatting

### **CSS Styling**
- Minimal custom CSS complementing Bootstrap
- Card hover effects and transitions
- Responsive design improvements
- Dark mode support (media query based)
- Loading state animations

## Usage

### **Access the Interface**
1. Start the Content Storage Server
2. Navigate to `http://localhost:8081/` (or your configured host/port)
3. The management interface will load automatically

### **Authentication**
- If server authentication is enabled (`ENABLE_AUTH=true`), you'll be prompted for an API key
- The API key can be provided via:
  - Browser prompt (automatic)
  - `X-API-Key` header
  - `api_key` query parameter

### **Navigation**
- **Server Health**: Monitor real-time server status
- **System Metrics**: View database and backup statistics
- **Management Operations**: Perform administrative tasks
- **Content Management**: Browse and manage stored content

## Configuration

The interface automatically adapts to server configuration:

- **Authentication**: Prompts for API key when required
- **Pagination**: Configurable items per page (default: 10)
- **Auto-refresh**: 30-second interval (configurable)
- **Error Handling**: Graceful degradation on API failures

## Browser Compatibility

- **Modern Browsers**: Chrome 90+, Firefox 88+, Safari 14+, Edge 90+
- **Mobile**: iOS Safari 14+, Chrome Mobile 90+
- **Features**: ES6+ JavaScript, CSS Grid, Flexbox

## Security Considerations

- **XSS Prevention**: HTML escaping for user content
- **CSRF Protection**: API key authentication
- **Content Security**: Bootstrap CDN with integrity checks
- **Input Validation**: Client-side validation with server-side enforcement

## Future Enhancements

- [ ] Content viewing modal with syntax highlighting
- [ ] Advanced search and filtering
- [ ] Real-time WebSocket updates
- [ ] Configuration management interface
- [ ] Log viewing and analysis
- [ ] Performance charts and graphs
- [ ] Backup management and restoration
- [ ] User management (multi-user support)

## Development

### **Adding New Features**
1. Add HTML structure to `index.html`
2. Implement JavaScript functions in `management.js`
3. Add custom styling to `custom.css` if needed
4. Update this README with new functionality

### **Testing**
- Test with authentication enabled/disabled
- Verify responsive design on mobile devices
- Test error handling with server offline
- Validate accessibility with screen readers

## Dependencies

### **External CDN Resources**
- Bootstrap 5.3.0 CSS and JS
- Bootstrap Icons 1.10.0
- No additional JavaScript libraries required

### **Server Dependencies**
- Content Storage Server with Echo framework
- Static file serving enabled
- CORS configured for web interface access
