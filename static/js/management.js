/**
 * Content Storage Server Management Interface
 * Modular JavaScript functions for server management
 */

// Configuration
const CONFIG = {
    baseUrl: window.location.origin,
    refreshInterval: 30000, // 30 seconds
    itemsPerPage: 10,
    apiKey: null // Will be set if authentication is enabled
};

// State management
const STATE = {
    currentPage: 1,
    totalItems: 0,
    searchQuery: '',
    autoRefresh: null,
    isLoadingContent: false
};

/**
 * Initialize the management interface
 */
function initializeManagement() {
    console.log('Initializing Content Storage Server Management Interface');

    // Check if authentication is required
    checkAuthentication();

    // Load initial data (health, metrics, backups - but NOT content)
    loadHealthData();
    loadMetrics();
    loadBackups();

    // Setup auto-refresh for health and metrics only
    setupAutoRefresh();

    // Setup event listeners
    setupEventListeners();

    // Show initial empty state for content
    showContentEmptyState();
}

/**
 * Check if authentication is required and prompt for API key if needed
 */
async function checkAuthentication() {
    try {
        const response = await fetch(`${CONFIG.baseUrl}/health`);
        if (response.status === 401) {
            const apiKey = prompt('Authentication required. Please enter your API key:');
            if (apiKey) {
                CONFIG.apiKey = apiKey;
            } else {
                showAlert('Authentication required but no API key provided', 'warning');
            }
        }
    } catch (error) {
        console.error('Error checking authentication:', error);
    }
}

/**
 * Setup event listeners
 */
function setupEventListeners() {
    // Search input
    const searchInput = document.getElementById('search-input');
    if (searchInput) {
        // Search on Enter key
        searchInput.addEventListener('keypress', function(e) {
            if (e.key === 'Enter') {
                performSearch();
            }
        });

        // Optional: Search as you type with debouncing
        let searchTimeout;
        searchInput.addEventListener('input', function() {
            clearTimeout(searchTimeout);
            searchTimeout = setTimeout(() => {
                // Only auto-search if there's text or if clearing the field
                if (this.value.trim().length >= 2 || this.value.trim().length === 0) {
                    performSearch();
                }
            }, 500); // 500ms delay
        });
    }
}

/**
 * Perform search operation
 */
function performSearch() {
    const searchInput = document.getElementById('search-input');
    if (searchInput) {
        STATE.searchQuery = searchInput.value.trim();
        STATE.currentPage = 1;

        // Update visual feedback
        updateSearchVisualFeedback();

        // Load content with search query
        loadContent();
    }
}

/**
 * Clear search and show empty state (since content is not auto-loaded)
 */
function clearSearch() {
    const searchInput = document.getElementById('search-input');
    if (searchInput) {
        searchInput.value = '';
        STATE.searchQuery = '';
        STATE.currentPage = 1;

        // Update visual feedback
        updateSearchVisualFeedback();

        // Show empty state instead of loading content automatically
        showContentEmptyState();
    }
}

/**
 * Update visual feedback for search state
 */
function updateSearchVisualFeedback() {
    const searchInput = document.getElementById('search-input');
    if (!searchInput) return;

    const isSearching = STATE.searchQuery && STATE.searchQuery.trim().length > 0;

    if (isSearching) {
        searchInput.classList.add('border-primary');
        searchInput.style.backgroundColor = '#f8f9ff';
    } else {
        searchInput.classList.remove('border-primary');
        searchInput.style.backgroundColor = '';
    }
}

/**
 * Setup auto-refresh functionality
 */
function setupAutoRefresh() {
    // Auto-refresh health and metrics only - content is manual only
    STATE.autoRefresh = setInterval(() => {
        loadHealthData();
        loadMetrics();
    }, CONFIG.refreshInterval);
}

/**
 * Create request headers with authentication if needed
 */
function getRequestHeaders() {
    const headers = {
        'Content-Type': 'application/json'
    };

    if (CONFIG.apiKey) {
        headers['X-API-Key'] = CONFIG.apiKey;
    }

    return headers;
}

/**
 * Make API request with error handling
 */
async function apiRequest(endpoint, options = {}) {
    try {
        const url = `${CONFIG.baseUrl}${endpoint}`;
        const requestOptions = {
            ...options,
            headers: {
                ...getRequestHeaders(),
                ...options.headers
            }
        };

        const response = await fetch(url, requestOptions);

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        return await response.json();
    } catch (error) {
        console.error(`API request failed for ${endpoint}:`, error);
        throw error;
    }
}

/**
 * Show alert message in navbar
 */
function showAlert(message, type = 'info', duration = 5000) {
    const alertContainer = document.getElementById('navbar-alert-container');
    if (!alertContainer) return;

    // Clear any existing alert
    alertContainer.innerHTML = '';

    const alertId = 'navbar-alert-' + Date.now();

    const alertHtml = `
        <div class="navbar-alert alert-${type} d-flex align-items-center" id="${alertId}">
            <i class="bi bi-${getAlertIcon(type)} me-2"></i>
            <span class="flex-grow-1">${message}</span>
            <button type="button" class="btn-close" onclick="dismissNavbarAlert()"></button>
        </div>
    `;

    alertContainer.innerHTML = alertHtml;

    // Auto-dismiss after duration
    if (duration > 0) {
        setTimeout(() => {
            dismissNavbarAlert();
        }, duration);
    }
}

/**
 * Dismiss navbar alert
 */
function dismissNavbarAlert() {
    const container = document.getElementById('navbar-alert-container');
    if (container) {
        container.innerHTML = '';
    }
}

/**
 * Get Bootstrap icon for alert type
 */
function getAlertIcon(type) {
    const icons = {
        'success': 'check-circle',
        'danger': 'exclamation-triangle',
        'warning': 'exclamation-triangle',
        'info': 'info-circle'
    };
    return icons[type] || 'info-circle';
}

/**
 * Update status badge with consistent styling
 */
function updateStatusBadge(element, text, isHealthy, extraClasses = '') {
    if (!element) return;
    element.textContent = text;
    const statusClass = isHealthy ? 'bg-success' : 'bg-warning';
    element.className = `badge ${extraClasses} ${statusClass}`;
}

/**
 * Load server health data
 */
async function loadHealthData() {
    try {
        const health = await apiRequest('/health');

        // Update health status with support for degraded state
        const statusElement = document.getElementById('health-status');
        const serverStatusElement = document.getElementById('server-status');
        const status = health.status || 'unknown';
        const isHealthy = status === 'healthy';
        const isDegraded = status === 'degraded';

        // Update main health status badge
        let badgeClass = 'bg-danger';
        if (isHealthy) badgeClass = 'bg-success';
        else if (isDegraded) badgeClass = 'bg-warning';

        if (statusElement) {
            statusElement.textContent = status.charAt(0).toUpperCase() + status.slice(1);
            statusElement.className = `badge small ${badgeClass}`;
        }

        // Update server status in navbar
        if (serverStatusElement) {
            let statusText = 'Offline';
            let statusClass = 'bg-danger';

            if (isHealthy) {
                statusText = 'Online';
                statusClass = 'bg-success';
            } else if (isDegraded) {
                statusText = 'Degraded';
                statusClass = 'bg-warning';
            }

            serverStatusElement.innerHTML = `<i class="bi bi-circle-fill"></i> ${statusText}`;
            serverStatusElement.className = `badge me-2 ${statusClass}`;
        }

        // Update uptime - handle both string and nanosecond formats
        const uptimeElement = document.getElementById('uptime');
        if (uptimeElement && health.uptime) {
            if (typeof health.uptime === 'string') {
                uptimeElement.textContent = formatDuration(health.uptime);
            } else {
                uptimeElement.textContent = formatDurationFromNanoseconds(health.uptime);
            }
        }

        // Update content count - handle nested metrics structure
        const countElement = document.getElementById('content-count');
        if (countElement) {
            let contentCount = 0;
            if (health.metrics) {
                if (typeof health.metrics.content_count === 'number') {
                    contentCount = health.metrics.content_count;
                } else if (health.metrics.content_count !== undefined) {
                    contentCount = parseInt(health.metrics.content_count) || 0;
                }
            }
            countElement.textContent = contentCount.toLocaleString();
        }

        // Update component health statuses if available (detailed health endpoint)
        updateComponentHealthStatuses(health.metrics);

        // Update last updated time
        const lastUpdatedElement = document.getElementById('last-updated');
        if (lastUpdatedElement) {
            lastUpdatedElement.textContent = new Date().toLocaleTimeString();
        }

    } catch (error) {
        console.error('Failed to load health data:', error);
        showAlert('Failed to load health data: ' + error.message, 'danger');

        // Update status to show error
        const statusElement = document.getElementById('health-status');
        const serverStatusElement = document.getElementById('server-status');

        if (statusElement) {
            statusElement.textContent = 'Error';
            statusElement.className = 'badge small bg-danger';
        }

        if (serverStatusElement) {
            serverStatusElement.innerHTML = '<i class="bi bi-circle-fill"></i> Offline';
            serverStatusElement.className = 'badge me-2 bg-danger';
        }
    }
}

/**
 * Update component health statuses from detailed health metrics
 */
function updateComponentHealthStatuses(metrics) {
    const componentHealthDiv = document.getElementById('component-health');

    if (!metrics || !componentHealthDiv) return;

    // Check if we have detailed component health data
    const hasDetailedHealth = metrics.database_status || metrics.backup_status ||
                             metrics.gc_status || metrics.overall_status;

    if (hasDetailedHealth) {
        // Show the component health section
        componentHealthDiv.style.display = 'block';

        // Update database status
        const dbStatusElement = document.getElementById('database-status');
        if (dbStatusElement && metrics.database_status) {
            const status = metrics.database_status;
            const badgeClass = getHealthBadgeClass(status);
            dbStatusElement.textContent = status.charAt(0).toUpperCase() + status.slice(1);
            dbStatusElement.className = `badge small ${badgeClass}`;
        }

        // Update backup status
        const backupStatusElement = document.getElementById('backup-status');
        if (backupStatusElement && metrics.backup_status) {
            const status = metrics.backup_status;
            const badgeClass = getHealthBadgeClass(status);
            backupStatusElement.textContent = status.charAt(0).toUpperCase() + status.slice(1);
            backupStatusElement.className = `badge small ${badgeClass}`;
        }

        // Update GC status
        const gcStatusElement = document.getElementById('gc-status');
        if (gcStatusElement && metrics.gc_status) {
            const status = metrics.gc_status;
            const badgeClass = getHealthBadgeClass(status);
            gcStatusElement.textContent = status.charAt(0).toUpperCase() + status.slice(1);
            gcStatusElement.className = `badge small ${badgeClass}`;
        }

        // Update overall status summary
        const overallStatusElement = document.getElementById('overall-status');
        if (overallStatusElement && metrics.overall_status) {
            const status = metrics.overall_status;
            const badgeClass = getHealthBadgeClass(status);
            overallStatusElement.textContent = status.charAt(0).toUpperCase() + status.slice(1);
            overallStatusElement.className = `badge small ${badgeClass}`;
        }
    } else {
        // Hide the component health section if no detailed data
        componentHealthDiv.style.display = 'none';
    }
}

/**
 * Get Bootstrap badge class for health status
 */
function getHealthBadgeClass(status) {
    switch (status?.toLowerCase()) {
        case 'healthy':
            return 'bg-success';
        case 'degraded':
            return 'bg-warning';
        case 'unhealthy':
            return 'bg-danger';
        default:
            return 'bg-secondary';
    }
}

/**
 * Load system metrics
 */
async function loadMetrics() {
    try {
        const metrics = await apiRequest('/api/v1/metrics');
        displayMetrics(metrics);
        displayQueueMetrics(metrics);
        displayAccessManagerStats(metrics);
    } catch (error) {
        console.error('Failed to load metrics:', error);
        const container = document.getElementById('metrics-container');
        if (container) {
            container.innerHTML = `
                <div class="col-12 text-center text-danger">
                    <i class="bi bi-exclamation-triangle"></i> Failed to load metrics: ${error.message}
                </div>
            `;
        }

        // Clear other metric containers on error
        clearMetricsContainers();
    }
}

/**
 * Display metrics in the UI
 */
function displayMetrics(metrics) {
    const container = document.getElementById('metrics-container');
    if (!container) return;

    // Extract GC stats for display - handle actual field names from BadgerStorage
    let gcLastRun = 'Never';
    let gcRunsCount = 0;
    if (metrics.gc_stats) {
        // Handle actual field names from GetGCStats()
        if (metrics.gc_stats.last_run_time) {
            gcLastRun = formatDate(metrics.gc_stats.last_run_time);
        } else if (metrics.gc_stats.last_run) {
            gcLastRun = formatDate(metrics.gc_stats.last_run);
        }

        if (metrics.gc_stats.total_runs) {
            gcRunsCount = metrics.gc_stats.total_runs;
        } else if (metrics.gc_stats.runs_count) {
            gcRunsCount = metrics.gc_stats.runs_count;
        }
    }

    const metricsHtml = `
        <div class="col-md-3 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Database Size</small>
                <span class="fw-bold small">${formatBytes(metrics.database_size_bytes || 0)}</span>
            </div>
        </div>
        <div class="col-md-3 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Health Status</small>
                <span class="badge small ${getHealthBadgeClass(metrics.health_status)}">${(metrics.health_status || 'unknown').charAt(0).toUpperCase() + (metrics.health_status || 'unknown').slice(1)}</span>
            </div>
        </div>
        <div class="col-md-3 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Backup Count</small>
                <span class="fw-bold small">${metrics.backup_count || 0}</span>
            </div>
        </div>
        <div class="col-md-3 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Last Backup</small>
                <span class="small">${metrics.last_backup ? formatDate(metrics.last_backup) : 'Never'}</span>
            </div>
        </div>
        <div class="col-md-3 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">GC Runs</small>
                <span class="fw-bold small">${gcRunsCount}</span>
            </div>
        </div>
        <div class="col-md-3 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Last GC</small>
                <span class="small">${gcLastRun}</span>
            </div>
        </div>
    `;

    container.innerHTML = metricsHtml;
}

/**
 * Display queue metrics in the UI
 */
function displayQueueMetrics(metrics) {
    const container = document.getElementById('queue-metrics-container');
    if (!container) return;

    // Check if queue metrics are available
    if (!metrics.queue_metrics) {
        container.innerHTML = `
            <div class="col-12 text-center text-muted">
                <small><i class="bi bi-info-circle"></i> Queue metrics not available</small>
            </div>
        `;
        return;
    }

    const queueMetrics = metrics.queue_metrics;

    // Determine queue health based on metrics
    let queueHealth = 'healthy';
    let queueHealthClass = 'bg-success';

    // Simple heuristics for queue health
    const queueDepth = queueMetrics.queue_depth || 0;
    const totalErrors = queueMetrics.total_errors || 0;
    const totalProcessed = queueMetrics.total_processed || 0;
    const errorRate = totalProcessed > 0 ? (totalErrors / totalProcessed) * 100 : 0;

    if (totalErrors > 0 && errorRate > 5) {
        queueHealth = 'unhealthy';
        queueHealthClass = 'bg-danger';
    } else if (queueDepth > 100 || (totalErrors > 0 && errorRate > 1)) {
        queueHealth = 'degraded';
        queueHealthClass = 'bg-warning';
    }

    const queueHtml = `
        <div class="col-md-4 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Queue Depth</small>
                <span class="fw-bold small ${queueDepth > 50 ? 'text-warning' : ''}">${queueDepth.toLocaleString()}</span>
            </div>
        </div>
        <div class="col-md-4 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Queue Health</small>
                <span class="badge small ${queueHealthClass}">${queueHealth.charAt(0).toUpperCase() + queueHealth.slice(1)}</span>
            </div>
        </div>
        <div class="col-md-4 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Total Processed</small>
                <span class="fw-bold small">${(queueMetrics.total_processed || 0).toLocaleString()}</span>
            </div>
        </div>
        <div class="col-md-4 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Total Queued</small>
                <span class="fw-bold small">${(queueMetrics.total_queued || 0).toLocaleString()}</span>
            </div>
        </div>
        <div class="col-md-4 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Queue Errors</small>
                <span class="fw-bold small ${totalErrors > 0 ? 'text-danger' : ''}">${totalErrors.toLocaleString()}</span>
            </div>
        </div>
        <div class="col-md-4 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Error Rate</small>
                <span class="fw-bold small ${errorRate > 1 ? 'text-warning' : ''}">${errorRate.toFixed(2)}%</span>
            </div>
        </div>
    `;

    container.innerHTML = queueHtml;
}

/**
 * Display access manager statistics
 */
function displayAccessManagerStats(metrics) {
    const container = document.getElementById('access-manager-container');
    if (!container) return;

    if (!metrics.access_manager_stats) {
        container.innerHTML = `
            <div class="col-12 text-center text-muted">
                <small>Access manager stats not available</small>
            </div>
        `;
        return;
    }

    const accessStats = metrics.access_manager_stats;

    const accessHtml = `
        <div class="col-md-6 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Tracked Content</small>
                <span class="fw-bold small">${(accessStats.tracked_content_count || 0).toLocaleString()}</span>
            </div>
        </div>
        <div class="col-md-6 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Total Accesses</small>
                <span class="fw-bold small">${(accessStats.total_accesses || 0).toLocaleString()}</span>
            </div>
        </div>
        <div class="col-md-6 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Cleanup Interval</small>
                <span class="small">${accessStats.cleanup_interval || 'Unknown'}</span>
            </div>
        </div>
        <div class="col-md-6 mb-2">
            <div class="text-center">
                <small class="text-muted d-block">Last Cleanup</small>
                <span class="small">${accessStats.last_cleanup_time ? formatDate(accessStats.last_cleanup_time) : 'Never'}</span>
            </div>
        </div>
    `;

    container.innerHTML = accessHtml;
}

/**
 * Clear metrics containers on error
 */
function clearMetricsContainers() {
    const containers = ['queue-metrics-container', 'access-manager-container'];
    containers.forEach(containerId => {
        const container = document.getElementById(containerId);
        if (container) {
            container.innerHTML = `
                <div class="col-12 text-center text-muted">
                    <small>Metrics unavailable</small>
                </div>
            `;
        }
    });
}

/**
 * Refresh all data (except content which is manual only)
 */
function refreshAll() {
    loadHealthData();
    loadMetrics();
    loadBackups();
}

/**
 * Load content data with pagination (with loading state)
 */
async function loadContent() {
    if (STATE.isLoadingContent) return; // Prevent concurrent loads

    STATE.isLoadingContent = true;
    showContentLoadingState();

    try {
        const limit = CONFIG.itemsPerPage;
        const offset = (STATE.currentPage - 1) * limit;

        // Build query parameters
        const params = new URLSearchParams({
            limit: limit.toString(),
            offset: offset.toString()
        });

        if (STATE.searchQuery) {
            params.append('search', STATE.searchQuery);
        }

        const content = await apiRequest(`/api/v1/content/?${params}`);
        const countData = await apiRequest('/api/v1/content/count');

        STATE.totalItems = countData.data?.count || 0;
        displayContent(content.data?.contents || []);
        updatePagination();
        updateContentTotalIndicator();

    } catch (error) {
        console.error('Failed to load content:', error);
        showContentErrorState(error.message);
    } finally {
        STATE.isLoadingContent = false;
    }
}



/**
 * Update content total indicator
 */
function updateContentTotalIndicator() {
    const totalCountElement = document.getElementById('content-total-count');
    if (totalCountElement) {
        totalCountElement.textContent = STATE.totalItems.toLocaleString();
    }
}



/**
 * Show content empty state (initial state)
 */
function showContentEmptyState() {
    const tbody = document.getElementById('content-table-body');
    if (tbody) {
        tbody.innerHTML = `
            <tr>
                <td colspan="7" class="text-center text-muted">
                    <i class="bi bi-inbox"></i> Click "Load Content" to fetch and display content
                    <br><small>Content is not loaded automatically</small>
                </td>
            </tr>
        `;
    }

    // Reset total count indicator
    const totalCountElement = document.getElementById('content-total-count');
    if (totalCountElement) {
        totalCountElement.textContent = '-';
    }
}

/**
 * Show content loading state
 */
function showContentLoadingState() {
    const tbody = document.getElementById('content-table-body');
    if (tbody) {
        tbody.innerHTML = `
            <tr>
                <td colspan="7" class="text-center text-muted">
                    <i class="bi bi-hourglass-split"></i> Loading content...
                </td>
            </tr>
        `;
    }
}

/**
 * Show content error state
 */
function showContentErrorState(errorMessage) {
    const tbody = document.getElementById('content-table-body');
    if (tbody) {
        tbody.innerHTML = `
            <tr>
                <td colspan="7" class="text-center text-danger">
                    <i class="bi bi-exclamation-triangle"></i> Failed to load content: ${errorMessage}
                    <br><small><a href="#" onclick="loadContent()" class="text-decoration-none">Click to retry</a></small>
                </td>
            </tr>
        `;
    }
}



/**
 * Display content in the table
 */
function displayContent(contentItems) {
    const tbody = document.getElementById('content-table-body');
    if (!tbody) return;

    if (!contentItems || contentItems.length === 0) {
        const isSearching = STATE.searchQuery && STATE.searchQuery.trim().length > 0;
        const message = isSearching
            ? `No content found matching "${STATE.searchQuery}"`
            : 'No content found';
        const icon = isSearching ? 'bi-search' : 'bi-inbox';

        tbody.innerHTML = `
            <tr>
                <td colspan="7" class="text-center text-muted">
                    <i class="bi ${icon}"></i> ${message}
                    ${isSearching ? '<br><small>Try a different search term or <a href="#" onclick="clearSearch()">clear search</a></small>' : ''}
                </td>
            </tr>
        `;
        return;
    }

    const rows = contentItems.map(item => `
        <tr>
            <td class="py-0 px-2">
                <code class="text-truncate-responsive" title="${escapeHtml(item.id)}">${escapeHtml(item.id)}</code>
            </td>
            <td class="py-0 px-2"><span class="badge bg-secondary small">${escapeHtml(item.type || 'unknown')}</span></td>
            <td class="py-0 px-2">${item.tag ? `<span class="badge bg-info small">${escapeHtml(item.tag)}</span>` : '-'}</td>
            <td class="py-0 px-2"><small>${formatDate(item.created_at)}</small></td>
            <td class="py-0 px-2"><small>${item.expires_at ? formatDate(item.expires_at) : 'Never'}</small></td>
            <td class="py-0 px-2">
                <span class="badge small ${item.access_count > 0 ? 'bg-success' : 'bg-light text-dark'}">
                    ${item.access_count || 0}
                </span>
            </td>
            <td class="py-0 px-2">
                <button class="btn text-danger px-1 py-0" style="font-size: 0.75rem; line-height: 1.2;"
                        onclick="deleteContent('${escapeHtml(item.id)}')" title="Delete content">
                    <i class="bi bi-trash" style="font-size: 0.7rem;"></i>
                </button>
                <button class="btn text-info ms-1 px-1 py-0" style="font-size: 0.75rem; line-height: 1.2;"
                        onclick="viewContent('${escapeHtml(item.id)}')" title="View content">
                    <i class="bi bi-eye" style="font-size: 0.7rem;"></i>
                </button>
            </td>
        </tr>
    `).join('');

    tbody.innerHTML = rows;
}

/**
 * Update pagination controls
 */
function updatePagination() {
    const pagination = document.getElementById('pagination');
    if (!pagination) return;

    const totalPages = Math.ceil(STATE.totalItems / CONFIG.itemsPerPage);

    if (totalPages <= 1) {
        pagination.innerHTML = '';
        return;
    }

    let paginationHtml = '';

    // Previous button
    paginationHtml += `
        <li class="page-item ${STATE.currentPage === 1 ? 'disabled' : ''}">
            <a class="page-link" href="#" onclick="changePage(${STATE.currentPage - 1})">Previous</a>
        </li>
    `;

    // Page numbers
    const startPage = Math.max(1, STATE.currentPage - 2);
    const endPage = Math.min(totalPages, STATE.currentPage + 2);

    for (let i = startPage; i <= endPage; i++) {
        paginationHtml += `
            <li class="page-item ${i === STATE.currentPage ? 'active' : ''}">
                <a class="page-link" href="#" onclick="changePage(${i})">${i}</a>
            </li>
        `;
    }

    // Next button
    paginationHtml += `
        <li class="page-item ${STATE.currentPage === totalPages ? 'disabled' : ''}">
            <a class="page-link" href="#" onclick="changePage(${STATE.currentPage + 1})">Next</a>
        </li>
    `;

    pagination.innerHTML = paginationHtml;
}

/**
 * Change page
 */
function changePage(page) {
    const totalPages = Math.ceil(STATE.totalItems / CONFIG.itemsPerPage);
    if (page < 1 || page > totalPages) return;

    STATE.currentPage = page;
    loadContent();
}

/**
 * Delete content item
 */
async function deleteContent(contentId) {
    if (!confirm(`Are you sure you want to delete content "${contentId}"?`)) {
        return;
    }

    try {
        await apiRequest(`/api/v1/content/${encodeURIComponent(contentId)}`, {
            method: 'DELETE'
        });

        showAlert(`Content "${contentId}" deleted successfully`, 'success');
        loadContent(); // Refresh the content list
        loadHealthData(); // Refresh health data to update count

    } catch (error) {
        console.error('Failed to delete content:', error);
        showAlert(`Failed to delete content: ${error.message}`, 'danger');
    }
}

/**
 * View content details (placeholder for future implementation)
 */
function viewContent(contentId) {
    showAlert(`View content feature not implemented yet for "${contentId}"`, 'info');
}

/**
 * Load available backups
 */
async function loadBackups() {
    try {
        const metrics = await apiRequest('/api/v1/metrics');

        // Extract backup information from metrics
        let backupFiles = [];
        let backupCount = 0;
        let lastBackup = null;

        if (metrics.backup_stats) {
            backupFiles = metrics.backup_stats.backup_files || [];
            backupCount = metrics.backup_count || 0;
            lastBackup = metrics.last_backup;
        }

        displayBackups(backupFiles, backupCount, lastBackup);

    } catch (error) {
        console.error('Failed to load backups:', error);
        const container = document.getElementById('backup-list');
        if (container) {
            container.innerHTML = `
                <div class="d-flex align-items-center justify-content-center h-100">
                    <div class="text-center text-danger">
                        <i class="bi bi-exclamation-triangle"></i> Failed to load backups: ${error.message}
                    </div>
                </div>
            `;
        }
    }
}

/**
 * Display backups in the UI
 */
function displayBackups(backupFiles, backupCount, lastBackup) {
    const container = document.getElementById('backup-list');
    if (!container) return;

    if (!backupFiles || backupFiles.length === 0) {
        container.innerHTML = `
            <div class="d-flex align-items-center justify-content-center h-100">
                <div class="text-center text-muted">
                    <i class="bi bi-archive"></i> No backups available
                    <div class="mt-2">
                        <small>Create your first backup using the "Create Backup" button above</small>
                    </div>
                </div>
            </div>
        `;
        return;
    }

    // Sort backup files by name (which includes timestamp) in descending order
    const sortedBackups = [...backupFiles].sort().reverse();

    let backupsHtml = `
        <div class="mb-2 p-2">
            <div class="d-flex justify-content-between align-items-center">
                <small class="text-muted">Total Backups:</small>
                <span class="badge bg-info small">${backupCount}</span>
            </div>
            ${lastBackup ? `
                <div class="d-flex justify-content-between align-items-center mt-1">
                    <small class="text-muted">Last Backup:</small>
                    <small>${formatDate(lastBackup)}</small>
                </div>
            ` : ''}
        </div>
        <hr class="my-1">
        <div class="backup-files">
    `;

    sortedBackups.forEach((backupPath, index) => {
        const fileName = backupPath.split('/').pop() || backupPath;
        const isRecent = index < 3; // Mark first 3 as recent

        backupsHtml += `
            <div class="backup-item ${isRecent ? 'border-success bg-light' : ''}">
                <div class="d-flex justify-content-between align-items-center">
                    <div class="flex-grow-1">
                        <div class="fw-bold text-truncate small" title="${escapeHtml(fileName)}">
                            ${escapeHtml(fileName)}
                        </div>
                        <small class="text-muted">
                            ${extractBackupTimestamp(fileName)}
                            ${isRecent ? '<span class="badge bg-success small ms-1">Recent</span>' : ''}
                        </small>
                    </div>
                    <div class="ms-1">
                        <button class="btn btn-sm btn-outline-secondary p-1"
                                onclick="showBackupDetails('${escapeHtml(backupPath)}')"
                                title="View backup details">
                            <i class="bi bi-info-circle"></i>
                        </button>
                    </div>
                </div>
            </div>
        `;
    });

    backupsHtml += '</div>';
    container.innerHTML = backupsHtml;
}

/**
 * Extract and format timestamp from backup filename
 */
function extractBackupTimestamp(fileName) {
    // Extract timestamp from filename like "content-storage-20240101-150405.backup"
    const match = fileName.match(/(\d{8})-(\d{6})/);
    if (match) {
        const dateStr = match[1]; // YYYYMMDD
        const timeStr = match[2]; // HHMMSS

        const year = dateStr.substring(0, 4);
        const month = dateStr.substring(4, 6);
        const day = dateStr.substring(6, 8);
        const hour = timeStr.substring(0, 2);
        const minute = timeStr.substring(2, 4);
        const second = timeStr.substring(4, 6);

        const date = new Date(`${year}-${month}-${day}T${hour}:${minute}:${second}`);
        return date.toLocaleString();
    }
    return 'Unknown date';
}

/**
 * Show backup details (placeholder for future implementation)
 */
function showBackupDetails(backupPath) {
    const fileName = backupPath.split('/').pop() || backupPath;
    showAlert(`Backup details for "${fileName}" - Feature coming soon!`, 'info');
}

/**
 * Trigger manual sync
 */
async function triggerSync(event) {
    const startTime = Date.now();
    const timestamp = new Date().toISOString();

    // Log sync operation initiation
    console.log(`[${timestamp}] SYNC OPERATION TRIGGERED:`, {
        endpoint: '/api/v1/sync',
        method: 'POST',
        userAgent: navigator.userAgent,
        timestamp: timestamp,
        operation: 'manual_sync'
    });

    try {
        const button = event.target;
        const originalText = button.innerHTML;
        button.innerHTML = '<i class="bi bi-hourglass-split"></i> Syncing...';
        button.disabled = true;

        console.log(`[${new Date().toISOString()}] Sending sync request to server...`);

        const response = await apiRequest('/api/v1/sync', { method: 'POST' });

        const duration = Date.now() - startTime;
        const completionTimestamp = new Date().toISOString();

        // Log successful completion
        console.log(`[${completionTimestamp}] SYNC OPERATION COMPLETED SUCCESSFULLY:`, {
            duration: `${duration}ms`,
            response: response,
            timestamp: completionTimestamp,
            operation: 'manual_sync',
            status: 'success'
        });

        // Add to visual log
        addOperationLog('SYNC', 'success', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/sync',
            response: response
        });

        showAlert('Sync operation completed successfully', 'success');

        button.innerHTML = originalText;
        button.disabled = false;

        // Refresh data after sync
        setTimeout(() => {
            refreshAll();
        }, 1000);

    } catch (error) {
        const duration = Date.now() - startTime;
        const errorTimestamp = new Date().toISOString();

        // Log error details
        console.error(`[${errorTimestamp}] SYNC OPERATION FAILED:`, {
            error: error.message,
            duration: `${duration}ms`,
            timestamp: errorTimestamp,
            operation: 'manual_sync',
            status: 'error',
            stack: error.stack
        });

        // Add to visual log
        addOperationLog('SYNC', 'error', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/sync',
            error: error.message
        });

        showAlert(`Failed to trigger sync: ${error.message}`, 'danger');

        // Reset button
        const button = event.target;
        button.innerHTML = '<i class="bi bi-arrow-repeat"></i> Trigger Sync';
        button.disabled = false;
    }
}

/**
 * Create manual backup
 */
async function createBackup(event) {
    const startTime = Date.now();
    const timestamp = new Date().toISOString();

    // Log backup operation initiation
    console.log(`[${timestamp}] BACKUP OPERATION TRIGGERED:`, {
        endpoint: '/api/v1/backup',
        method: 'POST',
        userAgent: navigator.userAgent,
        timestamp: timestamp,
        operation: 'manual_backup',
        requestBody: { immediate: true }
    });

    try {
        const button = event.target;
        const originalText = button.innerHTML;
        button.innerHTML = '<i class="bi bi-hourglass-split"></i> Creating...';
        button.disabled = true;

        console.log(`[${new Date().toISOString()}] Sending backup request to server...`);

        const response = await apiRequest('/api/v1/backup', {
            method: 'POST',
            body: JSON.stringify({ immediate: true })
        });

        const duration = Date.now() - startTime;
        const completionTimestamp = new Date().toISOString();

        // Log successful completion
        console.log(`[${completionTimestamp}] BACKUP OPERATION COMPLETED SUCCESSFULLY:`, {
            duration: `${duration}ms`,
            response: response,
            timestamp: completionTimestamp,
            operation: 'manual_backup',
            status: 'success',
            backupData: response.data || null
        });

        // Add to visual log
        addOperationLog('BACKUP', 'success', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/backup',
            response: response
        });

        showAlert('Backup created successfully', 'success');

        button.innerHTML = originalText;
        button.disabled = false;

        // Refresh metrics and backup list to show updated backup count
        setTimeout(() => {
            loadMetrics();
            loadBackups();
        }, 1000);

    } catch (error) {
        const duration = Date.now() - startTime;
        const errorTimestamp = new Date().toISOString();

        // Log error details
        console.error(`[${errorTimestamp}] BACKUP OPERATION FAILED:`, {
            error: error.message,
            duration: `${duration}ms`,
            timestamp: errorTimestamp,
            operation: 'manual_backup',
            status: 'error',
            stack: error.stack
        });

        // Add to visual log
        addOperationLog('BACKUP', 'error', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/backup',
            error: error.message
        });

        showAlert(`Failed to create backup: ${error.message}`, 'danger');

        // Reset button
        const button = event.target;
        button.innerHTML = '<i class="bi bi-archive"></i> Create Backup';
        button.disabled = false;
    }
}

/**
 * Run garbage collection
 */
async function runGarbageCollection(event) {
    const startTime = Date.now();
    const timestamp = new Date().toISOString();

    // Log GC operation initiation
    console.log(`[${timestamp}] GC OPERATION TRIGGERED:`, {
        endpoint: '/api/v1/gc',
        method: 'POST',
        userAgent: navigator.userAgent,
        timestamp: timestamp,
        operation: 'manual_gc'
    });

    try {
        const button = event.target;
        const originalText = button.innerHTML;
        button.innerHTML = '<i class="bi bi-hourglass-split"></i> Running GC...';
        button.disabled = true;

        console.log(`[${new Date().toISOString()}] Sending GC request to server...`);

        const response = await apiRequest('/api/v1/gc', { method: 'POST' });

        const duration = Date.now() - startTime;
        const completionTimestamp = new Date().toISOString();

        // Log successful completion
        console.log(`[${completionTimestamp}] GC OPERATION COMPLETED SUCCESSFULLY:`, {
            duration: `${duration}ms`,
            response: response,
            timestamp: completionTimestamp,
            operation: 'manual_gc',
            status: 'success'
        });

        // Add to visual log
        addOperationLog('GC', 'success', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/gc',
            response: response
        });

        showAlert('Garbage collection completed successfully', 'success');

        button.innerHTML = originalText;
        button.disabled = false;

        // Refresh metrics after GC
        setTimeout(() => {
            loadMetrics();
        }, 1000);

    } catch (error) {
        const duration = Date.now() - startTime;
        const errorTimestamp = new Date().toISOString();

        // Log error details
        console.error(`[${errorTimestamp}] GC OPERATION FAILED:`, {
            error: error.message,
            duration: `${duration}ms`,
            timestamp: errorTimestamp,
            operation: 'manual_gc',
            status: 'error',
            stack: error.stack
        });

        // Add to visual log
        addOperationLog('GC', 'error', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/gc',
            error: error.message
        });

        showAlert(`Failed to run garbage collection: ${error.message}`, 'danger');

        // Reset button
        const button = event.target;
        button.innerHTML = '<i class="bi bi-trash3"></i> Run GC';
        button.disabled = false;
    }
}

/**
 * Cleanup access trackers
 */
async function cleanupAccessTrackers(event) {
    const startTime = Date.now();
    const timestamp = new Date().toISOString();

    // Log cleanup operation initiation
    console.log(`[${timestamp}] CLEANUP OPERATION TRIGGERED:`, {
        endpoint: '/api/v1/cleanup',
        method: 'POST',
        userAgent: navigator.userAgent,
        timestamp: timestamp,
        operation: 'manual_cleanup'
    });

    try {
        const button = event.target;
        const originalText = button.innerHTML;
        button.innerHTML = '<i class="bi bi-hourglass-split"></i> Cleaning...';
        button.disabled = true;

        console.log(`[${new Date().toISOString()}] Sending cleanup request to server...`);

        const response = await apiRequest('/api/v1/cleanup', { method: 'POST' });

        const duration = Date.now() - startTime;
        const completionTimestamp = new Date().toISOString();

        // Log successful completion
        console.log(`[${completionTimestamp}] CLEANUP OPERATION COMPLETED SUCCESSFULLY:`, {
            duration: `${duration}ms`,
            response: response,
            timestamp: completionTimestamp,
            operation: 'manual_cleanup',
            status: 'success',
            removedTrackers: response.data?.removed_trackers || 0
        });

        // Add to visual log
        addOperationLog('CLEANUP', 'success', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/cleanup',
            response: response,
            removedTrackers: response.data?.removed_trackers || 0
        });

        const removedCount = response.data?.removed_trackers || 0;
        showAlert(`Access tracker cleanup completed successfully. Removed ${removedCount} stale trackers.`, 'success');

        button.innerHTML = originalText;
        button.disabled = false;

        // Refresh metrics after cleanup
        setTimeout(() => {
            loadMetrics();
        }, 1000);

    } catch (error) {
        const duration = Date.now() - startTime;
        const errorTimestamp = new Date().toISOString();

        // Log error details
        console.error(`[${errorTimestamp}] CLEANUP OPERATION FAILED:`, {
            error: error.message,
            duration: `${duration}ms`,
            timestamp: errorTimestamp,
            operation: 'manual_cleanup',
            status: 'error',
            stack: error.stack
        });

        // Add to visual log
        addOperationLog('CLEANUP', 'error', {
            duration: `${duration}ms`,
            endpoint: '/api/v1/cleanup',
            error: error.message
        });

        showAlert(`Failed to cleanup access trackers: ${error.message}`, 'danger');

        // Reset button
        const button = event.target;
        button.innerHTML = '<i class="bi bi-broom"></i> Cleanup Trackers';
        button.disabled = false;
    }
}

/**
 * Add operation log entry to the visual log display
 */
function addOperationLog(operation, status, details = {}) {
    const logsContainer = document.getElementById('operation-logs');
    if (!logsContainer) return;

    // Clear the placeholder message if it exists
    const placeholder = logsContainer.querySelector('.text-muted.text-center');
    if (placeholder) {
        placeholder.remove();
    }

    const timestamp = new Date().toLocaleString();
    const logEntry = document.createElement('div');
    logEntry.className = `log-entry ${status}`;

    let detailsHtml = '';
    if (details.duration) {
        detailsHtml += `<div class="details">Duration: ${details.duration}</div>`;
    }
    if (details.endpoint) {
        detailsHtml += `<div class="details">Endpoint: ${details.endpoint}</div>`;
    }
    if (details.error) {
        detailsHtml += `<div class="details">Error: ${details.error}</div>`;
    }
    if (details.response && details.response.message) {
        detailsHtml += `<div class="details">Response: ${details.response.message}</div>`;
    }
    if (details.removedTrackers !== undefined) {
        detailsHtml += `<div class="details">Removed Trackers: ${details.removedTrackers}</div>`;
    }

    logEntry.innerHTML = `
        <div>
            <span class="timestamp">[${timestamp}]</span>
            <span class="operation">${operation}</span>
            <span class="badge small bg-${status === 'success' ? 'success' : status === 'error' ? 'danger' : 'info'} ms-1">
                ${status.toUpperCase()}
            </span>
        </div>
        ${detailsHtml}
    `;

    // Add to the top of the logs
    logsContainer.insertBefore(logEntry, logsContainer.firstChild);

    // Limit to last 10 entries
    const entries = logsContainer.querySelectorAll('.log-entry');
    if (entries.length > 10) {
        entries[entries.length - 1].remove();
    }

    // Auto-scroll to top to show the new entry
    logsContainer.scrollTop = 0;
}

/**
 * Clear operation logs
 */
function clearOperationLogs() {
    const logsContainer = document.getElementById('operation-logs');
    if (!logsContainer) return;

    logsContainer.innerHTML = `
        <div class="text-muted text-center small p-2">
            <i class="bi bi-info-circle"></i> Operation logs will appear here when management operations are triggered.
        </div>
    `;
}

/**
 * Utility Functions
 */

/**
 * Format duration string (e.g., "1h30m45s")
 */
function formatDuration(duration) {
    if (!duration) return '-';

    // Parse duration string like "1h30m45.123s"
    const match = duration.match(/(?:(\d+)h)?(?:(\d+)m)?(?:(\d+(?:\.\d+)?)s)?/);
    if (!match) return duration;

    const hours = parseInt(match[1]) || 0;
    const minutes = parseInt(match[2]) || 0;
    const seconds = parseFloat(match[3]) || 0;

    const parts = [];
    if (hours > 0) parts.push(`${hours}h`);
    if (minutes > 0) parts.push(`${minutes}m`);
    if (seconds > 0) parts.push(`${Math.floor(seconds)}s`);

    return parts.length > 0 ? parts.join(' ') : '0s';
}

/**
 * Format duration from nanoseconds to human readable format
 */
function formatDurationFromNanoseconds(nanoseconds) {
    if (!nanoseconds || nanoseconds === 0) return '0s';

    // Convert nanoseconds to seconds
    const totalSeconds = Math.floor(nanoseconds / 1000000000);

    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;

    const parts = [];
    if (hours > 0) parts.push(`${hours}h`);
    if (minutes > 0) parts.push(`${minutes}m`);
    if (seconds > 0) parts.push(`${seconds}s`);

    return parts.length > 0 ? parts.join(' ') : '0s';
}

/**
 * Format bytes to human readable format
 */
function formatBytes(bytes) {
    if (bytes === 0) return '0 B';

    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/**
 * Format date to readable format
 */
function formatDate(dateString) {
    if (!dateString) return '-';

    try {
        const date = new Date(dateString);
        return date.toLocaleString();
    } catch (error) {
        return dateString;
    }
}

/**
 * Escape HTML to prevent XSS
 */
function escapeHtml(text) {
    if (!text) return '';

    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * Clean up when page is unloaded
 */
window.addEventListener('beforeunload', function() {
    if (STATE.autoRefresh) {
        clearInterval(STATE.autoRefresh);
    }
});
