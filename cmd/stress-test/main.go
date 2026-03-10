// Package main provides an enhanced stress test for the Content Storage Server
// that combines RPS (Requests Per Second) measurement with bug discovery capabilities.
//
// Bug Discovery Tests (inspired by test_client.py):
// 1. ID Validation Edge Cases - Path traversal patterns (.., .hidden, etc.)
// 2. Access Limit Boundary Tests - access_limit=0, 1 behavior
// 3. Concurrent Access Limit Exhaustion - Race condition detection
// 4. Expiration Edge Cases - Past/future expiration dates
// 5. Content Type Validation - Invalid content types
// 6. Large Content Handling - Size boundary tests
// 7. Rapid Create/Delete Cycles - Timing bugs
// 8. Data Integrity Under Load - Corruption detection
//
// This stress test aims to discover the same bugs as test_client.py:
// - Bug 1: ID Validation Inconsistency (IDs with .. can be stored but not retrieved)
// - Bug 2: Access Limit = 1 Behavior Incorrect
// - Bug 3: Access Limit = 0 Allows Unlimited Access
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// WorkloadPattern defines operation mix ratios
type WorkloadPattern string

const (
	WorkloadWriteOnly  WorkloadPattern = "write-only"  // 100% POST
	WorkloadReadOnly   WorkloadPattern = "read-only"   // 100% GET
	WorkloadBalanced   WorkloadPattern = "balanced"    // 50% read, 50% write
	WorkloadReadHeavy  WorkloadPattern = "read-heavy"  // 80% read, 20% write
	WorkloadWriteHeavy WorkloadPattern = "write-heavy" // 20% read, 80% write
	WorkloadCRUD       WorkloadPattern = "crud"        // POST 40%, GET 40%, DELETE 20%
)

// ContentSizeCategory defines content size ranges
type ContentSizeCategory string

const (
	SizeTiny   ContentSizeCategory = "tiny"   // 10-100 bytes
	SizeSmall  ContentSizeCategory = "small"  // 100B - 1KB
	SizeMedium ContentSizeCategory = "medium" // 1KB - 100KB
	SizeLarge  ContentSizeCategory = "large"  // 100KB - 1MB
)

// SizeDistribution weights for content generation
type SizeDistribution struct {
	Tiny, Small, Medium, Large float64
}

// OperationWeights defines the probability weights for each operation type
type OperationWeights struct {
	Post   float64
	Get    float64
	List   float64
	Delete float64
}

// GetWeights returns operation weights for a workload pattern
func (w WorkloadPattern) GetWeights() OperationWeights {
	switch w {
	case WorkloadWriteOnly:
		return OperationWeights{Post: 1.0, Get: 0, List: 0, Delete: 0}
	case WorkloadReadOnly:
		return OperationWeights{Post: 0, Get: 0.8, List: 0.2, Delete: 0}
	case WorkloadBalanced:
		return OperationWeights{Post: 0.5, Get: 0.4, List: 0.05, Delete: 0.05}
	case WorkloadReadHeavy:
		return OperationWeights{Post: 0.2, Get: 0.65, List: 0.1, Delete: 0.05}
	case WorkloadWriteHeavy:
		return OperationWeights{Post: 0.8, Get: 0.15, List: 0.0, Delete: 0.05}
	case WorkloadCRUD:
		return OperationWeights{Post: 0.4, Get: 0.4, List: 0.0, Delete: 0.2}
	default:
		return OperationWeights{Post: 1.0, Get: 0, List: 0, Delete: 0}
	}
}

// StressTestConfig holds configuration for the stress test
type StressTestConfig struct {
	ServerHost       string        // Server host (default: localhost)
	ServerPort       string        // Server port (default: 8082)
	Duration         time.Duration // Test duration (default: 30s)
	MaxConcurrency   int           // Maximum concurrent requests (default: 100)
	LaunchServer     bool          // Whether to launch server independently (default: true)
	ServerBinary     string        // Path to server binary (default: ./server)
	UseHTTPS         bool          // Whether to use HTTPS instead of HTTP (default: false)
	TLSSkipVerify    bool          // Whether to skip TLS certificate verification (default: false)
	ExportStats      bool          // Whether to export detailed statistics (default: true)
	VerboseProgress  bool          // Whether to show verbose progress reporting (default: false)
	FailureThreshold float64       // Failure rate threshold for early termination (default: 50.0)

	// New workload configuration
	WorkloadPattern    WorkloadPattern
	SizeDistribution   SizeDistribution
	EnableCleanup      bool
	CleanupConcurrency int

	// Bug discovery configuration
	BugDiscovery       BugDiscoveryConfig
}

// FailureDetail represents a detailed failure record
type FailureDetail struct {
	Timestamp    time.Time `json:"timestamp"`
	RequestID    int64     `json:"request_id"`
	StatusCode   int       `json:"status_code"`
	ErrorMessage string    `json:"error_message"`
	ErrorType    string    `json:"error_type"`
	Duration     int64     `json:"duration_ms"`
	Endpoint     string    `json:"endpoint"`
}

// BugDiscoveryConfig holds configuration for bug discovery tests
type BugDiscoveryConfig struct {
	Enabled                 bool
	IDValidationTests       bool
	AccessLimitTests        bool
	ExpirationTests         bool
	ConcurrentLimitTests    bool
	DataIntegrityTests      bool
	RapidCycleTests         bool
}

// BugDiscoveryResult tracks results of bug discovery tests
type BugDiscoveryResult struct {
	BugType         string    `json:"bug_type"`
	Severity        string    `json:"severity"`
	Description     string    `json:"description"`
	Details         string    `json:"details"`
	Timestamp       time.Time `json:"timestamp"`
	StatusCode      int       `json:"status_code"`
	ExpectedStatus  int       `json:"expected_status"`
}

// BugDiscoveryResults holds all discovered bugs
type BugDiscoveryResults struct {
	Bugs    []BugDiscoveryResult
	mutex   sync.RWMutex
}

// TimeSeriesPoint represents a point in time with metrics
type TimeSeriesPoint struct {
	Timestamp       time.Time `json:"timestamp"`
	TotalRequests   int64     `json:"total_requests"`
	SuccessfulReqs  int64     `json:"successful_requests"`
	FailedRequests  int64     `json:"failed_requests"`
	RequestsPerSec  float64   `json:"requests_per_second"`
	AvgResponseTime float64   `json:"avg_response_time_ms"`
}

// EndpointMetrics holds metrics for a specific endpoint
type EndpointMetrics struct {
	Endpoint          string         `json:"endpoint"`
	TotalRequests     int64          `json:"total_requests"`
	SuccessfulReqs    int64          `json:"successful_requests"`
	FailedReqs        int64          `json:"failed_requests"`
	TotalResponseTime int64          `json:"total_response_time_ms"`
	MinResponseTime   int64          `json:"min_response_time_ms"`
	MaxResponseTime   int64          `json:"max_response_time_ms"`
	StatusCodes       map[int]int64  `json:"status_codes"`
	mutex             sync.RWMutex   `json:"-"`
}

// TestMetrics holds comprehensive performance metrics for the stress test
type TestMetrics struct {
	// Basic counters
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	TotalResponseTime  int64 // in milliseconds
	RequestsPerSecond  float64

	// Response time statistics
	MinResponseTime    int64
	MaxResponseTime    int64
	ResponseTimes      []int64 // For percentile calculations

	// Detailed failure tracking
	FailureDetails     []FailureDetail
	FailuresByCode     map[int]int64
	FailuresByType     map[string]int64
	FailuresByEndpoint map[string]int64

	// Time series data for trend analysis
	TimeSeries         []TimeSeriesPoint

	// Concurrency tracking
	MaxConcurrentReqs  int64
	ConcurrentReqs     int64

	// Per-endpoint metrics
	EndpointMetrics    map[string]*EndpointMetrics

	mutex              sync.RWMutex
	startTime          time.Time
}

// ContentTracker tracks created content for reads and cleanup
type ContentTracker struct {
	IDs       []string
	mutex     sync.RWMutex
	maxLength int // Prevent memory issues
}

// NewContentTracker creates a new ContentTracker
func NewContentTracker(maxLength int) *ContentTracker {
	return &ContentTracker{
		IDs:       make([]string, 0, maxLength),
		maxLength: maxLength,
	}
}

// Add adds a content ID to the tracker
func (ct *ContentTracker) Add(id string) {
	ct.mutex.Lock()
	defer ct.mutex.Unlock()
	if len(ct.IDs) < ct.maxLength {
		ct.IDs = append(ct.IDs, id)
	}
}

// GetRandom returns a random content ID from the tracker
func (ct *ContentTracker) GetRandom() string {
	ct.mutex.RLock()
	defer ct.mutex.RUnlock()
	if len(ct.IDs) == 0 {
		return ""
	}
	return ct.IDs[rand.Intn(len(ct.IDs))]
}

// GetAll returns all tracked content IDs
func (ct *ContentTracker) GetAll() []string {
	ct.mutex.RLock()
	defer ct.mutex.RUnlock()
	ids := make([]string, len(ct.IDs))
	copy(ids, ct.IDs)
	return ids
}

// StressTest manages the stress testing process
type StressTest struct {
	config          *StressTestConfig
	metrics         *TestMetrics
	bugResults      *BugDiscoveryResults
	httpClient      *http.Client
	serverCmd       *exec.Cmd
	ctx             context.Context
	cancel          context.CancelFunc
	baseURL         string
	contentTracker  *ContentTracker
	rand            *rand.Rand
	randMutex       sync.Mutex
	// Track content IDs for access limit testing
	accessLimitIDs  map[string]int // contentID -> access_limit
	accessLimitMutex sync.RWMutex
}

func main() {
	// Parse command line flags
	config := parseFlags()

	// Display test configuration
	displayConfig(config)

	// Create stress test instance
	stressTest := NewStressTest(config)

	// Setup signal handling for graceful shutdown
	setupSignalHandling(stressTest)

	// Run the stress test
	if err := stressTest.Run(); err != nil {
		log.Fatalf("Stress test failed: %v", err)
	}
}

// parseFlags parses command line flags and returns configuration
func parseFlags() *StressTestConfig {
	config := &StressTestConfig{}

	flag.StringVar(&config.ServerHost, "host", "localhost", "Server host")
	flag.StringVar(&config.ServerPort, "port", "8082", "Server port")
	flag.DurationVar(&config.Duration, "duration", 30*time.Second, "Test duration")
	flag.IntVar(&config.MaxConcurrency, "concurrency", 100, "Maximum concurrent requests")
	flag.BoolVar(&config.LaunchServer, "launch-server", true, "Launch server independently")
	flag.StringVar(&config.ServerBinary, "server-binary", "./server", "Path to server binary")
	flag.BoolVar(&config.UseHTTPS, "https", false, "Use HTTPS instead of HTTP")
	flag.BoolVar(&config.TLSSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification")
	flag.BoolVar(&config.ExportStats, "export-stats", true, "Export detailed statistics to JSON/CSV files")
	flag.BoolVar(&config.VerboseProgress, "verbose", false, "Show verbose progress reporting")
	flag.Float64Var(&config.FailureThreshold, "failure-threshold", 50.0, "Failure rate threshold (%) for early termination")

	// New workload configuration flags
	flag.StringVar((*string)(&config.WorkloadPattern), "workload", "write-only",
		"Workload pattern: write-only, read-only, balanced, read-heavy, write-heavy, crud")
	flag.Float64Var(&config.SizeDistribution.Tiny, "size-tiny", 0.4, "Weight for tiny content (10-100 bytes)")
	flag.Float64Var(&config.SizeDistribution.Small, "size-small", 0.4, "Weight for small content (100B - 1KB)")
	flag.Float64Var(&config.SizeDistribution.Medium, "size-medium", 0.15, "Weight for medium content (1KB - 100KB)")
	flag.Float64Var(&config.SizeDistribution.Large, "size-large", 0.05, "Weight for large content (100KB - 1MB)")
	flag.BoolVar(&config.EnableCleanup, "cleanup", true, "Enable post-test data cleanup")
	flag.IntVar(&config.CleanupConcurrency, "cleanup-concurrency", 50, "Concurrent cleanup workers")

	// Bug discovery flags
	flag.BoolVar(&config.BugDiscovery.Enabled, "bug-discovery", true, "Enable bug discovery tests")
	flag.BoolVar(&config.BugDiscovery.IDValidationTests, "test-id-validation", true, "Test ID validation edge cases")
	flag.BoolVar(&config.BugDiscovery.AccessLimitTests, "test-access-limits", true, "Test access limit boundaries")
	flag.BoolVar(&config.BugDiscovery.ExpirationTests, "test-expiration", true, "Test expiration edge cases")
	flag.BoolVar(&config.BugDiscovery.ConcurrentLimitTests, "test-concurrent-limits", true, "Test concurrent access limit exhaustion")
	flag.BoolVar(&config.BugDiscovery.DataIntegrityTests, "test-data-integrity", true, "Test data integrity under load")
	flag.BoolVar(&config.BugDiscovery.RapidCycleTests, "test-rapid-cycles", true, "Test rapid create/delete cycles")

	flag.Parse()

	return config
}

// displayConfig shows the test configuration
func displayConfig(config *StressTestConfig) {
	fmt.Println("🚀 Content Storage Server - RPS Stress Test")
	fmt.Println(strings.Repeat("=", 50))

	protocol := "http"
	if config.UseHTTPS {
		protocol = "https"
	}
	fmt.Printf("Target Server: %s://%s:%s\n", protocol, config.ServerHost, config.ServerPort)
	fmt.Printf("Test Duration: %v\n", config.Duration)
	fmt.Printf("Max Concurrency: %d\n", config.MaxConcurrency)
	fmt.Printf("Launch Server: %t\n", config.LaunchServer)
	if config.UseHTTPS {
		fmt.Printf("HTTPS: enabled\n")
		fmt.Printf("TLS Skip Verify: %t\n", config.TLSSkipVerify)
	} else {
		fmt.Printf("HTTPS: disabled\n")
	}
	fmt.Printf("Export Stats: %t\n", config.ExportStats)
	fmt.Printf("Verbose Progress: %t\n", config.VerboseProgress)
	fmt.Printf("Failure Threshold: %.1f%%\n", config.FailureThreshold)

	// Display workload configuration
	fmt.Println("\n📊 Workload Configuration:")
	fmt.Printf("   Pattern: %s\n", config.WorkloadPattern)
	weights := config.WorkloadPattern.GetWeights()
	fmt.Printf("   Operations - POST: %.0f%%, GET: %.0f%%, LIST: %.0f%%, DELETE: %.0f%%\n",
		weights.Post*100, weights.Get*100, weights.List*100, weights.Delete*100)
	fmt.Printf("   Size Distribution - Tiny: %.0f%%, Small: %.0f%%, Medium: %.0f%%, Large: %.0f%%\n",
		config.SizeDistribution.Tiny*100, config.SizeDistribution.Small*100,
		config.SizeDistribution.Medium*100, config.SizeDistribution.Large*100)
	fmt.Printf("   Cleanup Enabled: %t (concurrency: %d)\n", config.EnableCleanup, config.CleanupConcurrency)

	// Display bug discovery configuration
	fmt.Println("\n🐛 Bug Discovery Configuration:")
	fmt.Printf("   Enabled: %t\n", config.BugDiscovery.Enabled)
	if config.BugDiscovery.Enabled {
		fmt.Printf("   ID Validation Tests: %t\n", config.BugDiscovery.IDValidationTests)
		fmt.Printf("   Access Limit Tests: %t\n", config.BugDiscovery.AccessLimitTests)
		fmt.Printf("   Expiration Tests: %t\n", config.BugDiscovery.ExpirationTests)
		fmt.Printf("   Concurrent Limit Tests: %t\n", config.BugDiscovery.ConcurrentLimitTests)
		fmt.Printf("   Data Integrity Tests: %t\n", config.BugDiscovery.DataIntegrityTests)
		fmt.Printf("   Rapid Cycle Tests: %t\n", config.BugDiscovery.RapidCycleTests)
	}

	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()
}

// NewStressTest creates a new stress test instance
func NewStressTest(config *StressTestConfig) *StressTest {
	ctx, cancel := context.WithCancel(context.Background())

	// Configure HTTP transport with optional TLS settings
	// High concurrency settings for stress testing
	transport := &http.Transport{
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     200,
		IdleConnTimeout:     90 * time.Second,
	}

	// Configure TLS if HTTPS is enabled
	if config.UseHTTPS {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: config.TLSSkipVerify,
		}
	}

	// Determine protocol and base URL
	protocol := "http"
	if config.UseHTTPS {
		protocol = "https"
	}

	// Initialize metrics with proper data structures
	metrics := &TestMetrics{
		MinResponseTime:    int64(^uint64(0) >> 1), // Max int64 value
		MaxResponseTime:    0,
		ResponseTimes:      make([]int64, 0, 10000), // Pre-allocate for performance
		FailureDetails:     make([]FailureDetail, 0, 1000),
		FailuresByCode:     make(map[int]int64),
		FailuresByType:     make(map[string]int64),
		FailuresByEndpoint: make(map[string]int64),
		TimeSeries:         make([]TimeSeriesPoint, 0, 100),
		EndpointMetrics:    make(map[string]*EndpointMetrics),
		startTime:          time.Now(),
	}

	// Initialize bug discovery results
	bugResults := &BugDiscoveryResults{
		Bugs: make([]BugDiscoveryResult, 0),
	}

	st := &StressTest{
		config:         config,
		metrics:        metrics,
		bugResults:     bugResults,
		httpClient:     &http.Client{Timeout: 30 * time.Second, Transport: transport},
		ctx:            ctx,
		cancel:         cancel,
		baseURL:        fmt.Sprintf("%s://%s:%s", protocol, config.ServerHost, config.ServerPort),
		contentTracker: NewContentTracker(100000), // Track up to 100k content IDs
		rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
		accessLimitIDs: make(map[string]int),
	}

	return st
}



// setupSignalHandling sets up graceful shutdown on interrupt signals
func setupSignalHandling(stressTest *StressTest) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n🛑 Received interrupt signal, shutting down gracefully...")
		stressTest.cancel()
	}()
}

// Run executes the complete stress test
func (st *StressTest) Run() error {
	fmt.Println("🔧 Starting stress test setup...")

	// Launch server if requested
	if st.config.LaunchServer {
		if err := st.launchServer(); err != nil {
			return fmt.Errorf("failed to launch server: %w", err)
		}
		defer st.shutdownServer()
	}

	// Wait for server to be ready
	if err := st.waitForServer(); err != nil {
		return fmt.Errorf("server not ready: %w", err)
	}

	// Start progress reporting
	go st.reportProgress()

	// Start time series collection
	go st.collectTimeSeries()

	// Run the actual stress test
	fmt.Println("🚀 Starting stress test...")
	startTime := time.Now()

	if err := st.executeStressTest(); err != nil {
		return fmt.Errorf("stress test execution failed: %w", err)
	}

	duration := time.Since(startTime)

	// Run bug discovery tests after stress test
	st.runBugDiscoveryTests()

	// Perform cleanup of test data
	st.performCleanup()

	// Generate comprehensive final report
	st.generateFinalReport(duration)

	// Export detailed statistics if requested
	if st.config.ExportStats {
		st.exportDetailedStats(duration)
	}

	return nil
}

// launchServer starts the server process independently
func (st *StressTest) launchServer() error {
	fmt.Printf("🚀 Launching server from: %s\n", st.config.ServerBinary)

	// Check if server binary exists
	if _, err := os.Stat(st.config.ServerBinary); os.IsNotExist(err) {
		// Try to build the server
		fmt.Println("📦 Server binary not found, attempting to build...")
		buildCmd := exec.Command("go", "build", "-o", st.config.ServerBinary, "./cmd/server")
		if output, err := buildCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build server: %w\nOutput: %s", err, output)
		}
		fmt.Println("✅ Server built successfully")
	}

	// Set environment variables for the server
	env := os.Environ()
	env = append(env, "ENABLE_AUTH=false") // Disable auth for stress testing
	env = append(env, "LOG_LEVEL=warn")    // Reduce logging overhead

	if st.config.UseHTTPS {
		// Configure HTTPS server
		env = append(env, "ENABLE_TLS=true")
		env = append(env, fmt.Sprintf("TLS_PORT=%s", st.config.ServerPort))
		env = append(env, fmt.Sprintf("HOST=%s", st.config.ServerHost))
		env = append(env, "TLS_CACHE_DIR=./stress-test-certs")
		// For stress testing, we'll allow any host (development mode)
		// In production, you should set TLS_HOSTS to specific domains
		fmt.Println("⚠️  HTTPS mode: Using development TLS configuration (any host allowed)")
	} else {
		// Configure HTTP server
		env = append(env, "ENABLE_TLS=false")
		env = append(env, fmt.Sprintf("PORT=%s", st.config.ServerPort))
		env = append(env, fmt.Sprintf("HOST=%s", st.config.ServerHost))
	}

	// Start the server
	st.serverCmd = exec.CommandContext(st.ctx, st.config.ServerBinary)
	st.serverCmd.Env = env
	st.serverCmd.Stdout = os.Stdout
	st.serverCmd.Stderr = os.Stderr

	if err := st.serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	fmt.Printf("✅ Server launched with PID: %d\n", st.serverCmd.Process.Pid)

	// Wait for warmup (longer for HTTPS due to certificate generation)
	if st.config.UseHTTPS {
		fmt.Printf("⏳ Waiting 10s for HTTPS server warmup (certificate generation)...\n")
		time.Sleep(10 * time.Second)
	} else {
		fmt.Printf("⏳ Waiting 3s for HTTP server warmup...\n")
		time.Sleep(3 * time.Second)
	}

	return nil
}

// shutdownServer gracefully shuts down the server
func (st *StressTest) shutdownServer() {
	if st.serverCmd != nil && st.serverCmd.Process != nil {
		fmt.Println("🛑 Shutting down server...")

		// Send interrupt signal
		if err := st.serverCmd.Process.Signal(syscall.SIGINT); err != nil {
			log.Printf("Failed to send interrupt signal: %v", err)
		}

		// Wait for graceful shutdown with timeout
		done := make(chan error, 1)
		go func() {
			done <- st.serverCmd.Wait()
		}()

		select {
		case <-done:
			fmt.Println("✅ Server shut down gracefully")
		case <-time.After(10 * time.Second):
			fmt.Println("⚠️ Server shutdown timeout, forcing kill...")
			if err := st.serverCmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill server process: %v", err)
			}
		}
	}
}

// waitForServer waits for the server to be ready to accept requests
func (st *StressTest) waitForServer() error {
	fmt.Println("⏳ Waiting for server to be ready...")

	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		resp, err := st.httpClient.Get(st.baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Println("✅ Server is ready!")
				return nil
			}
		}

		if i < maxAttempts-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return fmt.Errorf("server not ready after %d attempts", maxAttempts)
}

// executeStressTest runs the main stress test
func (st *StressTest) executeStressTest() error {
	// Create a context with timeout for the test duration
	testCtx, testCancel := context.WithTimeout(st.ctx, st.config.Duration)
	defer testCancel()

	// Channel to control concurrency
	semaphore := make(chan struct{}, st.config.MaxConcurrency)

	// WaitGroup to track all requests
	var wg sync.WaitGroup

	// Request counter for generating unique IDs
	var requestCounter int64

	// Main request generation loop
	for {
		select {
		case <-testCtx.Done():
			// Test duration reached
			fmt.Println("⏰ Test duration reached, waiting for remaining requests...")
			wg.Wait()
			return nil
		case <-st.ctx.Done():
			// Interrupted
			fmt.Println("🛑 Test interrupted, waiting for remaining requests...")
			wg.Wait()
			return nil
		default:
			// Try to acquire semaphore (non-blocking)
			select {
			case semaphore <- struct{}{}:
				wg.Add(1)
				reqID := atomic.AddInt64(&requestCounter, 1)

				// Launch request goroutine
				go func(reqID int64) {
					defer func() {
						<-semaphore
						wg.Done()
					}()

					st.executeRequest(reqID)
				}(reqID)
			default:
				// Semaphore full, yield CPU briefly
				time.Sleep(time.Microsecond)
			}
		}
	}
}

// executeRequest executes a single API request based on workload pattern
func (st *StressTest) executeRequest(requestID int64) {
	startTime := time.Now()

	// Track concurrent requests
	atomic.AddInt64(&st.metrics.ConcurrentReqs, 1)
	defer atomic.AddInt64(&st.metrics.ConcurrentReqs, -1)

	// Update max concurrent requests if needed
	current := atomic.LoadInt64(&st.metrics.ConcurrentReqs)
	for {
		max := atomic.LoadInt64(&st.metrics.MaxConcurrentReqs)
		if current <= max || atomic.CompareAndSwapInt64(&st.metrics.MaxConcurrentReqs, max, current) {
			break
		}
	}

	// Select operation based on workload pattern
	operation := st.selectOperation()

	var statusCode int
	var err error
	var endpoint string

	switch operation {
	case "POST":
		statusCode, err, endpoint = st.executePostContent(requestID)
	case "GET":
		statusCode, err, endpoint = st.executeGetContent()
	case "LIST":
		statusCode, err, endpoint = st.executeListContent()
	case "DELETE":
		statusCode, err, endpoint = st.executeDeleteContent()
	default:
		statusCode, err, endpoint = st.executePostContent(requestID)
	}

	duration := time.Since(startTime)
	st.recordMetrics(requestID, duration, statusCode, err, endpoint)
}

// selectOperation selects an operation based on workload pattern weights
func (st *StressTest) selectOperation() string {
	weights := st.config.WorkloadPattern.GetWeights()
	r := st.randomFloat64()
	cumulative := 0.0

	if weights.Post > 0 {
		cumulative += weights.Post
		if r < cumulative {
			return "POST"
		}
	}
	if weights.Get > 0 {
		cumulative += weights.Get
		if r < cumulative {
			return "GET"
		}
	}
	if weights.List > 0 {
		cumulative += weights.List
		if r < cumulative {
			return "LIST"
		}
	}
	if weights.Delete > 0 {
		cumulative += weights.Delete
		if r < cumulative {
			return "DELETE"
		}
	}

	return "POST" // Default fallback
}

// randomFloat64 returns a thread-safe random float64 in [0.0, 1.0)
func (st *StressTest) randomFloat64() float64 {
	st.randMutex.Lock()
	defer st.randMutex.Unlock()
	return st.rand.Float64()
}

// randomIntn returns a thread-safe random integer in [0, n)
func (st *StressTest) randomIntn(n int) int {
	if n <= 0 {
		return 0
	}
	st.randMutex.Lock()
	defer st.randMutex.Unlock()
	return st.rand.Intn(n)
}

// selectSizeCategory selects a content size category based on distribution weights
func (st *StressTest) selectSizeCategory() ContentSizeCategory {
	r := st.randomFloat64()
	cumulative := 0.0

	cumulative += st.config.SizeDistribution.Tiny
	if r < cumulative {
		return SizeTiny
	}

	cumulative += st.config.SizeDistribution.Small
	if r < cumulative {
		return SizeSmall
	}

	cumulative += st.config.SizeDistribution.Medium
	if r < cumulative {
		return SizeMedium
	}

	return SizeLarge
}

// generateSizeBytes generates a random size in bytes for a given category
func (st *StressTest) generateSizeBytes(category ContentSizeCategory) int {
	switch category {
	case SizeTiny:
		return 10 + st.randomIntn(90) // 10-100 bytes
	case SizeSmall:
		return 100 + st.randomIntn(900) // 100B - 1KB
	case SizeMedium:
		return 1024 + st.randomIntn(100*1024) // 1KB - 100KB
	case SizeLarge:
		return 100*1024 + st.randomIntn(900*1024) // 100KB - 1MB
	default:
		return 20
	}
}

// generateData generates random data of the specified size
func (st *StressTest) generateData(sizeBytes int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "
	result := make([]byte, sizeBytes)
	for i := range result {
		result[i] = charset[st.randomIntn(len(charset))]
	}
	return string(result)
}

// executePostContent performs a POST request to store content
func (st *StressTest) executePostContent(requestID int64) (int, error, string) {
	endpoint := "POST /api/v1/content"

	// Select size based on distribution
	category := st.selectSizeCategory()
	sizeBytes := st.generateSizeBytes(category)

	// Generate content
	contentID := fmt.Sprintf("stress-%d-%d", requestID, time.Now().UnixNano())
	content := map[string]any{
		"id":   contentID,
		"data": st.generateData(sizeBytes),
		"type": "text/plain",
		"tag":  string(category),
	}

	jsonData, err := json.Marshal(content)
	if err != nil {
		return 0, err, endpoint
	}

	resp, err := st.httpClient.Post(
		st.baseURL+"/api/v1/content",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		return 0, err, endpoint
	}
	defer resp.Body.Close()

	// Read response body to ensure complete processing
	io.Copy(io.Discard, resp.Body)

	// Track successful content creation for reads and cleanup
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		st.contentTracker.Add(contentID)
	}

	return resp.StatusCode, nil, endpoint
}

// executeGetContent performs GET /api/v1/content/:id
func (st *StressTest) executeGetContent() (int, error, string) {
	endpoint := "GET /api/v1/content/:id"

	id := st.contentTracker.GetRandom()
	if id == "" {
		// No content to read, skip this request
		return 0, fmt.Errorf("no content available for read"), endpoint
	}

	resp, err := st.httpClient.Get(st.baseURL + "/api/v1/content/" + id)
	if err != nil {
		return 0, err, endpoint
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil, endpoint
}

// executeListContent performs GET /api/v1/content
func (st *StressTest) executeListContent() (int, error, string) {
	endpoint := "GET /api/v1/content"

	resp, err := st.httpClient.Get(st.baseURL + "/api/v1/content?limit=10")
	if err != nil {
		return 0, err, endpoint
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil, endpoint
}

// executeDeleteContent performs DELETE /api/v1/content/:id
func (st *StressTest) executeDeleteContent() (int, error, string) {
	endpoint := "DELETE /api/v1/content/:id"

	id := st.contentTracker.GetRandom()
	if id == "" {
		// No content to delete, skip this request
		return 0, fmt.Errorf("no content available for delete"), endpoint
	}

	req, err := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+id, nil)
	if err != nil {
		return 0, err, endpoint
	}

	resp, err := st.httpClient.Do(req)
	if err != nil {
		return 0, err, endpoint
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil, endpoint
}

// runBugDiscoveryTests runs all enabled bug discovery tests
func (st *StressTest) runBugDiscoveryTests() {
	if !st.config.BugDiscovery.Enabled {
		fmt.Println("⏭️  Bug discovery tests disabled")
		return
	}

	fmt.Println("\n🔍 Starting Bug Discovery Tests...")
	fmt.Println(strings.Repeat("=", 50))

	if st.config.BugDiscovery.IDValidationTests {
		st.testIDValidationEdgeCases()
	}

	if st.config.BugDiscovery.AccessLimitTests {
		st.testAccessLimitBoundaries()
	}

	if st.config.BugDiscovery.ExpirationTests {
		st.testExpirationEdgeCases()
	}

	if st.config.BugDiscovery.ConcurrentLimitTests {
		st.testConcurrentAccessLimitExhaustion()
	}

	if st.config.BugDiscovery.DataIntegrityTests {
		st.testDataIntegrityUnderLoad()
	}

	if st.config.BugDiscovery.RapidCycleTests {
		st.testRapidCreateDeleteCycles()
	}

	// Generate bug discovery report
	st.generateBugDiscoveryReport()
}

// recordBug records a discovered bug
func (st *StressTest) recordBug(bugType, severity, description, details string, statusCode, expectedStatus int) {
	st.bugResults.mutex.Lock()
	defer st.bugResults.mutex.Unlock()

	st.bugResults.Bugs = append(st.bugResults.Bugs, BugDiscoveryResult{
		BugType:        bugType,
		Severity:       severity,
		Description:    description,
		Details:        details,
		Timestamp:      time.Now(),
		StatusCode:     statusCode,
		ExpectedStatus: expectedStatus,
	})
}

// waitForContent polls the server until content is available or timeout
// Uses /status endpoint to avoid incrementing access count
func (st *StressTest) waitForContent(id string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Use /status endpoint which doesn't increment access count (uses GetReadOnly)
		resp, err := st.httpClient.Get(st.baseURL + "/api/v1/content/" + id + "/status")
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 200 {
			// Parse status response
			var result map[string]interface{}
			if err := json.Unmarshal(body, &result); err == nil {
				if data, ok := result["data"].(map[string]interface{}); ok {
					if status, ok := data["status"].(string); ok && status == "stored" {
						return true
					}
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// testIDValidationEdgeCases tests Bug 1: ID Validation Inconsistency
// IDs with '..' can be stored but not retrieved
func (st *StressTest) testIDValidationEdgeCases() {
	fmt.Println("\n🐛 Testing ID Validation Edge Cases...")

	testIDs := []struct {
		id          string
		description string
	}{
		{"..hidden", "leading double dot"},
		{"file..txt", "double dot in middle"},
		{"test..", "trailing double dot"},
		{".hidden", "leading single dot"},
		{"file.", "trailing single dot"},
		{"../etc/passwd", "path traversal"},
		{"..\\windows\\", "windows path traversal"},
		{"file%2e%2e", "url encoded dots"},
		{"normal_id", "normal ID (should work)"},
	}

	for _, test := range testIDs {
		// Try to store content with this ID
		content := map[string]any{
			"id":   test.id,
			"data": "test data for ID validation",
			"type": "text/plain",
		}

		jsonData, _ := json.Marshal(content)
		resp, err := st.httpClient.Post(
			st.baseURL+"/api/v1/content",
			"application/json",
			strings.NewReader(string(jsonData)),
		)
		if err != nil {
			fmt.Printf("   ❌ Store failed for '%s': %v\n", test.description, err)
			continue
		}
		defer resp.Body.Close()
		storeStatus := resp.StatusCode

		// Try to retrieve the content
		getResp, getErr := st.httpClient.Get(st.baseURL + "/api/v1/content/" + test.id)
		if getErr != nil {
			fmt.Printf("   ❌ Get failed for '%s': %v\n", test.description, getErr)
			continue
		}
		defer getResp.Body.Close()
		getStatus := getResp.StatusCode

		// Check for the bug: stored successfully but cannot retrieve
		if storeStatus == 202 && getStatus == 400 {
			st.recordBug(
				"ID Validation Inconsistency",
				"HIGH",
				fmt.Sprintf("ID '%s' (%s) stored successfully but cannot be retrieved", test.id, test.description),
				fmt.Sprintf("store_status=%d, get_status=%d", storeStatus, getStatus),
				getStatus,
				200,
			)
			fmt.Printf("   🐛 BUG FOUND: '%s' stored (202) but get returned %d\n", test.description, getStatus)
		} else if storeStatus == 202 && getStatus == 200 {
			fmt.Printf("   ✅ '%s': stored and retrieved successfully\n", test.description)
		} else if storeStatus == 400 && getStatus == 404 {
			fmt.Printf("   ✅ '%s': rejected at store (consistent)\n", test.description)
		} else {
			fmt.Printf("   ⚠️  '%s': store=%d, get=%d (unexpected)\n", test.description, storeStatus, getStatus)
		}

		// Cleanup
		if storeStatus == 202 {
			req, _ := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+test.id, nil)
			st.httpClient.Do(req)
		}
	}
}

// testAccessLimitBoundaries tests Bug 2 & 3: Access limit boundary issues
func (st *StressTest) testAccessLimitBoundaries() {
	fmt.Println("\n🐛 Testing Access Limit Boundaries...")

	// Test access_limit = 1 (Bug 2)
	fmt.Println("   Testing access_limit=1...")
	content1 := map[string]any{
		"id":           fmt.Sprintf("access-test-1-%d", time.Now().UnixNano()),
		"data":         "test data",
		"type":         "text/plain",
		"access_limit": 1,
	}

	jsonData, err := json.Marshal(content1)
	if err != nil {
		fmt.Printf("   ❌ Failed to marshal content1: %v\n", err)
		return
	}
	resp, err := st.httpClient.Post(
		st.baseURL+"/api/v1/content",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		fmt.Printf("   ❌ POST failed for access_limit=1: %v\n", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode == 202 {
		// Wait for content to be stored (poll status endpoint)
		if !st.waitForContent(content1["id"].(string), 2*time.Second) {
			fmt.Printf("   ⚠️  Content not available after 2s, skipping test\n")
			return
		}

		// First access - should succeed
		getResp1, err := st.httpClient.Get(st.baseURL + "/api/v1/content/" + content1["id"].(string))
		if err != nil {
			fmt.Printf("   ❌ GET failed for access_limit=1: %v\n", err)
		} else {
			io.Copy(io.Discard, getResp1.Body)
			status1 := getResp1.StatusCode
			getResp1.Body.Close()

			if status1 == 410 {
				st.recordBug(
					"Access Limit = 1 Behavior",
					"HIGH",
					"access_limit=1 returns 410 on first access (should allow 1 access)",
					fmt.Sprintf("first_status=%d, expected=200", status1),
					status1,
					200,
				)
				fmt.Printf("   🐛 BUG FOUND: access_limit=1 returned 410 on first access\n")
			} else if status1 == 200 {
				fmt.Printf("   ✅ access_limit=1: first access returned 200 (correct)\n")

				// Second access - should fail
				getResp2, err := st.httpClient.Get(st.baseURL + "/api/v1/content/" + content1["id"].(string))
				if err == nil {
					io.Copy(io.Discard, getResp2.Body)
					status2 := getResp2.StatusCode
					getResp2.Body.Close()
					if status2 == 410 {
						fmt.Printf("   ✅ access_limit=1: second access returned 410 (correct)\n")
					} else {
						fmt.Printf("   ⚠️  access_limit=1: second access returned %d (expected 410)\n", status2)
					}
				}
			} else {
				fmt.Printf("   ⚠️  access_limit=1: first access returned %d (unexpected)\n", status1)
			}
		}

		// Cleanup
		req, _ := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+content1["id"].(string), nil)
		st.httpClient.Do(req)
	}

	// Test access_limit = 0 (Bug 3)
	fmt.Println("   Testing access_limit=0...")
	content0 := map[string]any{
		"id":           fmt.Sprintf("access-test-0-%d", time.Now().UnixNano()),
		"data":         "test data",
		"type":         "text/plain",
		"access_limit": 0,
	}

	jsonData0, err := json.Marshal(content0)
	if err != nil {
		fmt.Printf("   ❌ Failed to marshal content0: %v\n", err)
		return
	}
	resp0, err := st.httpClient.Post(
		st.baseURL+"/api/v1/content",
		"application/json",
		strings.NewReader(string(jsonData0)),
	)
	if err != nil {
		fmt.Printf("   ❌ POST failed for access_limit=0: %v\n", err)
		return
	}
	io.Copy(io.Discard, resp0.Body)
	storeStatus := resp0.StatusCode
	resp0.Body.Close()

	if storeStatus == 202 {
		// Wait for content to be stored (poll status endpoint)
		if !st.waitForContent(content0["id"].(string), 2*time.Second) {
			fmt.Printf("   ⚠️  Content not available after 2s, skipping test\n")
			return
		}

		// access_limit=0 means "no access allowed" - content should return 410 immediately
		getResp, err := st.httpClient.Get(st.baseURL + "/api/v1/content/" + content0["id"].(string))
		if err == nil {
			io.Copy(io.Discard, getResp.Body)
			status := getResp.StatusCode
			getResp.Body.Close()

			if status == 410 {
				fmt.Printf("   ✅ access_limit=0: first access returned 410 (correct - no access allowed)\n")
			} else if status == 200 {
				// BUG: access_limit=0 should mean "no access allowed", not "unlimited access"
				st.recordBug(
					"Access Limit = 0 Behavior",
					"MEDIUM",
					"access_limit=0 allows access when it should mean 'no access allowed'",
					fmt.Sprintf("first access returned 200, expected 410"),
					200,
					410,
				)
				fmt.Printf("   🐛 BUG FOUND: access_limit=0 allows access (expected 410, got 200)\n")
			} else {
				fmt.Printf("   ⚠️  access_limit=0: first access returned %d (unexpected)\n", status)
			}
		}
	}

	// Cleanup
	req, _ := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+content0["id"].(string), nil)
	st.httpClient.Do(req)
}

// testExpirationEdgeCases tests expiration date boundaries
func (st *StressTest) testExpirationEdgeCases() {
	fmt.Println("\n🐛 Testing Expiration Edge Cases...")

	now := time.Now()

	testCases := []struct {
		name       string
		expiration time.Time
		expected   int // Expected GET status
	}{
		{"past date", now.Add(-1 * time.Hour), 404},
		{"now", now, 404},
		{"1 second ago", now.Add(-1 * time.Second), 404},
		{"1 second future", now.Add(1 * time.Second), 200},
		{"future date", now.Add(1 * time.Hour), 200},
	}

	for _, tc := range testCases {
		id := fmt.Sprintf("expire-test-%d", time.Now().UnixNano())
		content := map[string]any{
			"id":         id,
			"data":       "test data",
			"type":       "text/plain",
			"expiration": tc.expiration.Format(time.RFC3339),
		}

		jsonData, _ := json.Marshal(content)
		resp, _ := st.httpClient.Post(
			st.baseURL+"/api/v1/content/",
			"application/json",
			strings.NewReader(string(jsonData)),
		)
		if resp != nil {
			resp.Body.Close()
			// Wait for async store
			time.Sleep(100 * time.Millisecond)

			getResp, _ := st.httpClient.Get(st.baseURL + "/api/v1/content/" + id)
			if getResp != nil {
				status := getResp.StatusCode
				getResp.Body.Close()

				if status != tc.expected {
					// Report as issue, not necessarily bug
					fmt.Printf("   ⚠️  '%s': expected %d, got %d\n", tc.name, tc.expected, status)
				} else {
					fmt.Printf("   ✅ '%s': got expected %d\n", tc.name, status)
				}
			}

			// Cleanup
			req, _ := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+id, nil)
			st.httpClient.Do(req)
		}
	}
}

// testConcurrentAccessLimitExhaustion tests race conditions with access limits
func (st *StressTest) testConcurrentAccessLimitExhaustion() {
	fmt.Println("\n🐛 Testing Concurrent Access Limit Exhaustion...")

	id := fmt.Sprintf("concurrent-limit-%d", time.Now().UnixNano())
	accessLimit := 5

	content := map[string]any{
		"id":           id,
		"data":         "test data for concurrent access",
		"type":         "text/plain",
		"access_limit": accessLimit,
	}

	jsonData, err := json.Marshal(content)
	if err != nil {
		fmt.Printf("   ❌ Failed to marshal content: %v\n", err)
		return
	}
	resp, err := st.httpClient.Post(
		st.baseURL+"/api/v1/content",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		fmt.Printf("   ❌ POST failed for concurrent test: %v\n", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 202 {
		fmt.Printf("   ⚠️  Unexpected store status: %d\n", resp.StatusCode)
		return
	}

	// Wait for content to be stored (poll status endpoint)
	if !st.waitForContent(id, 2*time.Second) {
		fmt.Printf("   ⚠️  Content not available after 2s, skipping concurrent test\n")
		return
	}

	// Spawn concurrent requests
	concurrency := 20
	var wg sync.WaitGroup
	successCount := int64(0)
	goneCount := int64(0)
	notFoundCount := int64(0)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			getResp, err := st.httpClient.Get(st.baseURL + "/api/v1/content/" + id)
			if err != nil {
				return
			}
			io.Copy(io.Discard, getResp.Body)
			status := getResp.StatusCode
			getResp.Body.Close()

			switch status {
			case 200:
				atomic.AddInt64(&successCount, 1)
			case 410:
				atomic.AddInt64(&goneCount, 1)
			case 404:
				atomic.AddInt64(&notFoundCount, 1)
			}
		}()
	}

	wg.Wait()

	// Note: With truly concurrent requests, we may not get exactly accessLimit successes
	// The test checks that we get AT MOST accessLimit successes (not more)
	// And that total successes + gone equals the number of requests that completed
	totalAccounted := successCount + goneCount + notFoundCount
	fmt.Printf("   📊 Results: successes=%d, gone=%d, not_found=%d, total_accounted=%d\n",
		successCount, goneCount, notFoundCount, totalAccounted)

	// Bug: If we got MORE than accessLimit successes, that's a real bug
	if int(successCount) > accessLimit {
		st.recordBug(
			"Concurrent Access Limit Exceeded",
			"HIGH",
			"Concurrent access allowed MORE successes than access_limit",
			fmt.Sprintf("expected at most %d successes, got %d", accessLimit, successCount),
			int(successCount),
			accessLimit,
		)
		fmt.Printf("   🐛 BUG FOUND: Got %d successes (more than limit %d)\n", successCount, accessLimit)
	} else if int(successCount) == 0 && int(goneCount) == 0 && int(notFoundCount) > 0 {
		// Not a bug - content expired before concurrent test ran
		fmt.Printf("   ⚠️  Content expired before test (all 404s)\n")
	} else {
		fmt.Printf("   ✅ Concurrent access: at most %d successes (limit %d)\n", successCount, accessLimit)
	}

	// Cleanup
	req, _ := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+id, nil)
	st.httpClient.Do(req)
}

// testDataIntegrityUnderLoad tests for data corruption under load
func (st *StressTest) testDataIntegrityUnderLoad() {
	fmt.Println("\n🐛 Testing Data Integrity Under Load...")

	// Store content with known data
	id := fmt.Sprintf("integrity-%d", time.Now().UnixNano())
	originalData := "INTEGRITY_TEST_DATA_12345"

	content := map[string]any{
		"id":   id,
		"data": originalData,
		"type": "text/plain",
	}

	jsonData, err := json.Marshal(content)
	if err != nil {
		fmt.Printf("   ❌ Failed to marshal content: %v\n", err)
		return
	}
	resp, err := st.httpClient.Post(
		st.baseURL+"/api/v1/content",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		fmt.Printf("   ❌ POST failed for data integrity test: %v\n", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 202 {
		fmt.Printf("   ⚠️  Unexpected store status: %d\n", resp.StatusCode)
		return
	}

	// Wait for content to be stored (poll status endpoint)
	if !st.waitForContent(id, 2*time.Second) {
		fmt.Printf("   ⚠️  Content not available after 2s, skipping test\n")
		return
	}

	// Concurrent reads to verify integrity
	concurrency := 10
	iterations := 10
	var wg sync.WaitGroup
	mismatchCount := int64(0)
	successReads := int64(0)
	notFoundCount := int64(0)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				getResp, err := st.httpClient.Get(st.baseURL + "/api/v1/content/" + id)
				if err != nil {
					continue
				}

				body, _ := io.ReadAll(getResp.Body)
				getResp.Body.Close()

				switch getResp.StatusCode {
				case 200:
					atomic.AddInt64(&successReads, 1)
					var result map[string]any
					if err := json.Unmarshal(body, &result); err == nil {
						if data, ok := result["data"].(string); ok {
							if data != originalData {
								atomic.AddInt64(&mismatchCount, 1)
							}
						}
					}
				case 404:
					atomic.AddInt64(&notFoundCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	if mismatchCount > 0 {
		st.recordBug(
			"Data Integrity",
			"HIGH",
			"Data integrity violation under concurrent load",
			fmt.Sprintf("%d mismatches detected out of %d reads", mismatchCount, successReads),
			0,
			0,
		)
		fmt.Printf("   🐛 BUG FOUND: %d data integrity violations\n", mismatchCount)
	} else {
		fmt.Printf("   ✅ Data integrity verified under load (%d reads, %d not found)\n", successReads, notFoundCount)
	}

	// Cleanup
	req, _ := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+id, nil)
	st.httpClient.Do(req)
}

// testRapidCreateDeleteCycles tests for timing issues with rapid operations
func (st *StressTest) testRapidCreateDeleteCycles() {
	fmt.Println("\n🐛 Testing Rapid Create/Delete Cycles...")

	cycles := 10
	errors := 0

	for i := 0; i < cycles; i++ {
		id := fmt.Sprintf("rapid-%d-%d", i, time.Now().UnixNano())

		// Create
		content := map[string]any{
			"id":   id,
			"data": "test data",
			"type": "text/plain",
		}

		jsonData, _ := json.Marshal(content)
		resp, err := st.httpClient.Post(
			st.baseURL+"/api/v1/content",
			"application/json",
			strings.NewReader(string(jsonData)),
		)
		if err != nil || resp.StatusCode != 202 {
			errors++
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		resp.Body.Close()

		// Immediate delete (before async store completes)
		req, _ := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+id, nil)
		deleteResp, _ := st.httpClient.Do(req)
		if deleteResp != nil {
			deleteResp.Body.Close()
		}

		// Try to read - might get 404 or 200 depending on timing
		getResp, _ := st.httpClient.Get(st.baseURL + "/api/v1/content/" + id)
		if getResp != nil {
			getResp.Body.Close()
		}
	}

	fmt.Printf("   ✅ Completed %d rapid cycles (errors: %d)\n", cycles, errors)
}

// generateBugDiscoveryReport generates a report of discovered bugs
func (st *StressTest) generateBugDiscoveryReport() {
	st.bugResults.mutex.RLock()
	defer st.bugResults.mutex.RUnlock()

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🐛 BUG DISCOVERY REPORT")
	fmt.Println(strings.Repeat("=", 80))

	if len(st.bugResults.Bugs) == 0 {
		fmt.Println("\n✅ No bugs discovered in this run!")
		return
	}

	fmt.Printf("\nTotal Bugs Discovered: %d\n\n", len(st.bugResults.Bugs))

	// Group by severity
	severityCount := make(map[string]int)
	for _, bug := range st.bugResults.Bugs {
		severityCount[bug.Severity]++
	}

	fmt.Println("Severity Distribution:")
	for sev, count := range severityCount {
		icon := "⚠️"
		switch sev {
		case "HIGH":
			icon = "🔴"
		case "MEDIUM":
			icon = "🟡"
		case "LOW":
			icon = "🟢"
		}
		fmt.Printf("   %s %s: %d\n", icon, sev, count)
	}

	fmt.Println("\nDiscovered Bugs:")
	for i, bug := range st.bugResults.Bugs {
		fmt.Printf("\n%d. [%s] %s\n", i+1, bug.Severity, bug.BugType)
		fmt.Printf("   Description: %s\n", bug.Description)
		fmt.Printf("   Details: %s\n", bug.Details)
		fmt.Printf("   Timestamp: %s\n", bug.Timestamp.Format(time.RFC3339))
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
}

// performCleanup deletes all test content created during the stress test
func (st *StressTest) performCleanup() {
	if !st.config.EnableCleanup {
		fmt.Println("⏭️  Skipping cleanup (disabled)")
		return
	}

	ids := st.contentTracker.GetAll()
	if len(ids) == 0 {
		fmt.Println("⏭️  No content to clean up")
		return
	}

	fmt.Printf("\n🧹 Cleaning up %d content items...\n", len(ids))
	startTime := time.Now()

	semaphore := make(chan struct{}, st.config.CleanupConcurrency)
	var wg sync.WaitGroup
	var deleted, failed int64

	for _, id := range ids {
		// Acquire semaphore BEFORE spawning goroutine to limit concurrent goroutines
		semaphore <- struct{}{}
		wg.Add(1)
		go func(contentID string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			req, err := http.NewRequest("DELETE", st.baseURL+"/api/v1/content/"+contentID, nil)
			if err != nil {
				atomic.AddInt64(&failed, 1)
				return
			}

			resp, err := st.httpClient.Do(req)
			if err != nil {
				atomic.AddInt64(&failed, 1)
				return
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
				atomic.AddInt64(&deleted, 1)
			} else {
				atomic.AddInt64(&failed, 1)
			}
		}(id)
	}

	wg.Wait()

	elapsed := time.Since(startTime)
	if failed > 0 {
		fmt.Printf("⚠️  Cleanup completed: %d deleted, %d failed in %v\n", deleted, failed, elapsed.Round(time.Millisecond))
	} else {
		fmt.Printf("✅ Cleanup completed: %d items deleted in %v\n", deleted, elapsed.Round(time.Millisecond))
	}
}

// recordMetrics records comprehensive performance metrics for a request
func (st *StressTest) recordMetrics(requestID int64, duration time.Duration, statusCode int, err error, endpoint string) {
	st.metrics.mutex.Lock()
	defer st.metrics.mutex.Unlock()

	// Update total metrics
	atomic.AddInt64(&st.metrics.TotalRequests, 1)

	durationMs := duration.Nanoseconds() / 1000000 // Convert to milliseconds
	atomic.AddInt64(&st.metrics.TotalResponseTime, durationMs)

	// Update response time statistics
	if durationMs < st.metrics.MinResponseTime {
		st.metrics.MinResponseTime = durationMs
	}
	if durationMs > st.metrics.MaxResponseTime {
		st.metrics.MaxResponseTime = durationMs
	}

	// Store response time for percentile calculations (limit to prevent memory issues)
	if len(st.metrics.ResponseTimes) < 10000 {
		st.metrics.ResponseTimes = append(st.metrics.ResponseTimes, durationMs)
	}

	// Determine if request was successful
	isSuccess := err == nil && statusCode >= 200 && statusCode < 300

	if isSuccess {
		atomic.AddInt64(&st.metrics.SuccessfulRequests, 1)
	} else {
		atomic.AddInt64(&st.metrics.FailedRequests, 1)

		// Record detailed failure information
		st.recordFailureDetails(requestID, statusCode, err, durationMs, endpoint)
	}

	// Record per-endpoint metrics
	st.recordEndpointMetrics(endpoint, durationMs, statusCode, err)
}

// recordEndpointMetrics records metrics for a specific endpoint
func (st *StressTest) recordEndpointMetrics(endpoint string, durationMs int64, statusCode int, err error) {
	metrics := st.metrics.EndpointMetrics[endpoint]
	if metrics == nil {
		metrics = &EndpointMetrics{
			Endpoint:        endpoint,
			MinResponseTime: int64(^uint64(0) >> 1),
			MaxResponseTime: 0,
			StatusCodes:     make(map[int]int64),
		}
		st.metrics.EndpointMetrics[endpoint] = metrics
	}

	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()

	metrics.TotalRequests++
	metrics.TotalResponseTime += durationMs
	metrics.StatusCodes[statusCode]++

	if durationMs < metrics.MinResponseTime {
		metrics.MinResponseTime = durationMs
	}
	if durationMs > metrics.MaxResponseTime {
		metrics.MaxResponseTime = durationMs
	}

	if err == nil && statusCode >= 200 && statusCode < 300 {
		metrics.SuccessfulReqs++
	} else {
		metrics.FailedReqs++
	}
}

// recordFailureDetails records detailed information about a failed request
func (st *StressTest) recordFailureDetails(requestID int64, statusCode int, err error, durationMs int64, endpoint string) {
	// Determine error type and message
	var errorType, errorMessage string

	if err != nil {
		errorMessage = err.Error()
		// Categorize error types
		switch {
		case strings.Contains(errorMessage, "timeout"):
			errorType = "timeout"
		case strings.Contains(errorMessage, "connection refused"):
			errorType = "connection_refused"
		case strings.Contains(errorMessage, "connection reset"):
			errorType = "connection_reset"
		case strings.Contains(errorMessage, "no such host"):
			errorType = "dns_error"
		case strings.Contains(errorMessage, "TLS"):
			errorType = "tls_error"
		case strings.Contains(errorMessage, "EOF"):
			errorType = "connection_closed"
		default:
			errorType = "network_error"
		}
	} else {
		errorMessage = fmt.Sprintf("HTTP %d", statusCode)
		// Categorize HTTP status codes
		switch {
		case statusCode >= 400 && statusCode < 500:
			errorType = "client_error"
		case statusCode >= 500:
			errorType = "server_error"
		default:
			errorType = "unknown_error"
		}
	}

	// Create failure detail record
	failureDetail := FailureDetail{
		Timestamp:    time.Now(),
		RequestID:    requestID,
		StatusCode:   statusCode,
		ErrorMessage: errorMessage,
		ErrorType:    errorType,
		Duration:     durationMs,
		Endpoint:     endpoint,
	}

	// Store failure detail (limit to prevent memory issues)
	if len(st.metrics.FailureDetails) < 1000 {
		st.metrics.FailureDetails = append(st.metrics.FailureDetails, failureDetail)
	}

	// Update failure counters
	st.metrics.FailuresByCode[statusCode]++
	st.metrics.FailuresByType[errorType]++
	st.metrics.FailuresByEndpoint[endpoint]++
}



// collectTimeSeries collects time series data for trend analysis
func (st *StressTest) collectTimeSeries() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			st.metrics.mutex.Lock()

			totalReqs := atomic.LoadInt64(&st.metrics.TotalRequests)
			successReqs := atomic.LoadInt64(&st.metrics.SuccessfulRequests)
			failedReqs := atomic.LoadInt64(&st.metrics.FailedRequests)

			elapsed := time.Since(st.metrics.startTime)
			rps := float64(totalReqs) / elapsed.Seconds()

			var avgResponseTime float64
			if totalReqs > 0 {
				totalResponseTime := atomic.LoadInt64(&st.metrics.TotalResponseTime)
				avgResponseTime = float64(totalResponseTime) / float64(totalReqs)
			}

			point := TimeSeriesPoint{
				Timestamp:       time.Now(),
				TotalRequests:   totalReqs,
				SuccessfulReqs:  successReqs,
				FailedRequests:  failedReqs,
				RequestsPerSec:  rps,
				AvgResponseTime: avgResponseTime,
			}

			// Limit time series data to prevent memory issues
			if len(st.metrics.TimeSeries) < 100 {
				st.metrics.TimeSeries = append(st.metrics.TimeSeries, point)
			}

			st.metrics.mutex.Unlock()
		}
	}
}

// reportProgress periodically reports test progress with enhanced metrics
func (st *StressTest) reportProgress() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-st.ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := st.config.Duration - elapsed

			if remaining <= 0 {
				return
			}

			// Calculate current metrics
			totalReqs := atomic.LoadInt64(&st.metrics.TotalRequests)
			successReqs := atomic.LoadInt64(&st.metrics.SuccessfulRequests)
			failedReqs := atomic.LoadInt64(&st.metrics.FailedRequests)
			concurrentReqs := atomic.LoadInt64(&st.metrics.ConcurrentReqs)

			rps := float64(totalReqs) / elapsed.Seconds()
			var successRate float64
			if totalReqs > 0 {
				successRate = float64(successReqs) / float64(totalReqs) * 100
			}

			var avgResponseTime float64
			if totalReqs > 0 {
				totalResponseTime := atomic.LoadInt64(&st.metrics.TotalResponseTime)
				avgResponseTime = float64(totalResponseTime) / float64(totalReqs)
			}

			// Check failure threshold for early termination
			if totalReqs > 100 && successRate < (100.0-st.config.FailureThreshold) {
				fmt.Printf("\n⚠️  EARLY TERMINATION: Failure rate %.1f%% exceeds threshold %.1f%%\n",
					100.0-successRate, st.config.FailureThreshold)
				st.cancel()
				return
			}

			// Display progress based on verbosity setting
			if st.config.VerboseProgress {
				fmt.Printf("📊 [%v] RPS: %.1f | Success: %.1f%% | Failed: %d | Concurrent: %d | Avg RT: %.1fms | Remaining: %v\n",
					elapsed.Round(time.Second),
					rps,
					successRate,
					failedReqs,
					concurrentReqs,
					avgResponseTime,
					remaining.Round(time.Second))
			} else {
				fmt.Printf("📊 RPS: %.1f | Success: %.1f%% | Failed: %d | Remaining: %v\n",
					rps,
					successRate,
					failedReqs,
					remaining.Round(time.Second))
			}
		}
	}
}

// generateFinalReport generates and displays a comprehensive final test report
func (st *StressTest) generateFinalReport(duration time.Duration) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🎯 COMPREHENSIVE STRESS TEST FINAL REPORT")
	fmt.Println(strings.Repeat("=", 80))

	// Calculate final metrics
	totalReqs := atomic.LoadInt64(&st.metrics.TotalRequests)
	successReqs := atomic.LoadInt64(&st.metrics.SuccessfulRequests)
	failedReqs := atomic.LoadInt64(&st.metrics.FailedRequests)
	maxConcurrent := atomic.LoadInt64(&st.metrics.MaxConcurrentReqs)

	var rps float64
	var successRate float64
	if totalReqs > 0 {
		rps = float64(totalReqs) / duration.Seconds()
		successRate = float64(successReqs) / float64(totalReqs) * 100
	}

	// Basic Performance Metrics
	fmt.Println("\n📈 PERFORMANCE METRICS")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("🚀 Requests Per Second:    %.1f RPS\n", rps)
	fmt.Printf("⏱️  Test Duration:          %v\n", duration.Round(time.Millisecond))
	fmt.Printf("📊 Total Requests:         %d\n", totalReqs)
	fmt.Printf("✅ Successful Requests:    %d (%.1f%%)\n", successReqs, successRate)
	fmt.Printf("❌ Failed Requests:        %d (%.1f%%)\n", failedReqs, 100-successRate)
	fmt.Printf("🔄 Max Concurrent:         %d\n", maxConcurrent)
	fmt.Printf("📋 Workload Pattern:       %s\n", st.config.WorkloadPattern)

	// Response Time Statistics
	st.generateResponseTimeReport()

	// Per-Endpoint Performance
	st.generateEndpointReport()

	// Detailed Failure Analysis
	if failedReqs > 0 {
		st.generateFailureAnalysisReport()
	}

	// Performance Assessment
	fmt.Println("\n🔍 PERFORMANCE ASSESSMENT")
	fmt.Println(strings.Repeat("-", 50))

	if rps >= 1000 {
		fmt.Printf("🏆 EXCELLENT RPS: %.1f >= 1000 RPS\n", rps)
	} else if rps >= 500 {
		fmt.Printf("✅ GOOD RPS: %.1f >= 500 RPS\n", rps)
	} else if rps >= 100 {
		fmt.Printf("⚠️  FAIR RPS: %.1f >= 100 RPS\n", rps)
	} else {
		fmt.Printf("❌ POOR RPS: %.1f < 100 RPS\n", rps)
	}

	if successRate >= 99.0 {
		fmt.Printf("🏆 EXCELLENT SUCCESS RATE: %.1f%% (Excellent)\n", successRate)
	} else if successRate >= 95.0 {
		fmt.Printf("✅ GOOD SUCCESS RATE: %.1f%% (Good)\n", successRate)
	} else if successRate >= 90.0 {
		fmt.Printf("⚠️  FAIR SUCCESS RATE: %.1f%% (Needs attention)\n", successRate)
	} else {
		fmt.Printf("❌ POOR SUCCESS RATE: %.1f%% (Critical issue)\n", successRate)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("🎯 FINAL RESULT: %.1f RPS with %.1f%% success rate\n", rps, successRate)
	fmt.Println(strings.Repeat("=", 80))
}

// generateEndpointReport generates per-endpoint performance breakdown
func (st *StressTest) generateEndpointReport() {
	fmt.Println("\n📊 PER-ENDPOINT PERFORMANCE")
	fmt.Println(strings.Repeat("-", 60))

	if len(st.metrics.EndpointMetrics) == 0 {
		fmt.Println("No endpoint data available")
		return
	}

	for endpoint, metrics := range st.metrics.EndpointMetrics {
		if metrics.TotalRequests == 0 {
			continue
		}

		avgTime := float64(metrics.TotalResponseTime) / float64(metrics.TotalRequests)
		var successRate float64
		if metrics.TotalRequests > 0 {
			successRate = float64(metrics.SuccessfulReqs) / float64(metrics.TotalRequests) * 100
		}

		fmt.Printf("\n🎯 %s\n", endpoint)
		fmt.Printf("   Requests: %d | Success: %.1f%% | Avg: %.1fms | Min: %dms | Max: %dms\n",
			metrics.TotalRequests, successRate, avgTime, metrics.MinResponseTime, metrics.MaxResponseTime)

		// Show status code distribution
		if len(metrics.StatusCodes) > 0 {
			fmt.Printf("   Status Codes: ")
			codes := make([]string, 0, len(metrics.StatusCodes))
			for code, count := range metrics.StatusCodes {
				codes = append(codes, fmt.Sprintf("%d:%d", code, count))
			}
			fmt.Println(strings.Join(codes, ", "))
		}
	}
}

// generateResponseTimeReport generates detailed response time statistics
func (st *StressTest) generateResponseTimeReport() {
	st.metrics.mutex.RLock()
	defer st.metrics.mutex.RUnlock()

	fmt.Println("\n⚡ RESPONSE TIME STATISTICS")
	fmt.Println(strings.Repeat("-", 50))

	totalReqs := atomic.LoadInt64(&st.metrics.TotalRequests)
	if totalReqs == 0 {
		fmt.Println("No requests completed")
		return
	}

	totalResponseTime := atomic.LoadInt64(&st.metrics.TotalResponseTime)
	avgResponseTime := float64(totalResponseTime) / float64(totalReqs)

	fmt.Printf("📊 Average Response Time:  %.1f ms\n", avgResponseTime)
	fmt.Printf("⚡ Minimum Response Time:  %d ms\n", st.metrics.MinResponseTime)
	fmt.Printf("🐌 Maximum Response Time:  %d ms\n", st.metrics.MaxResponseTime)

	// Calculate percentiles if we have response time data
	if len(st.metrics.ResponseTimes) > 0 {
		st.calculateAndDisplayPercentiles()
	}
}

// calculateAndDisplayPercentiles calculates and displays response time percentiles
func (st *StressTest) calculateAndDisplayPercentiles() {
	// Make a copy and sort for percentile calculation
	times := make([]int64, len(st.metrics.ResponseTimes))
	copy(times, st.metrics.ResponseTimes)

	// Simple sort (for small datasets)
	for i := 0; i < len(times); i++ {
		for j := i + 1; j < len(times); j++ {
			if times[i] > times[j] {
				times[i], times[j] = times[j], times[i]
			}
		}
	}

	percentiles := []float64{50, 75, 90, 95, 99}
	fmt.Println("\n📈 Response Time Percentiles:")

	for _, p := range percentiles {
		index := int(float64(len(times)-1) * p / 100.0)
		if index >= len(times) {
			index = len(times) - 1
		}
		fmt.Printf("   P%.0f: %d ms\n", p, times[index])
	}
}

// generateFailureAnalysisReport generates detailed failure analysis
func (st *StressTest) generateFailureAnalysisReport() {
	st.metrics.mutex.RLock()
	defer st.metrics.mutex.RUnlock()

	fmt.Println("\n❌ DETAILED FAILURE ANALYSIS")
	fmt.Println(strings.Repeat("-", 50))

	totalFailed := atomic.LoadInt64(&st.metrics.FailedRequests)
	fmt.Printf("Total Failed Requests: %d\n", totalFailed)

	// Failure by Type Analysis
	if len(st.metrics.FailuresByType) > 0 {
		fmt.Println("\n🔍 Failures by Type:")
		for errorType, count := range st.metrics.FailuresByType {
			percentage := float64(count) / float64(totalFailed) * 100
			fmt.Printf("   %s: %d (%.1f%%)\n", errorType, count, percentage)
		}
	}

	// Failure by HTTP Status Code Analysis
	if len(st.metrics.FailuresByCode) > 0 {
		fmt.Println("\n📊 Failures by HTTP Status Code:")
		for statusCode, count := range st.metrics.FailuresByCode {
			percentage := float64(count) / float64(totalFailed) * 100
			fmt.Printf("   HTTP %d: %d (%.1f%%)\n", statusCode, count, percentage)
		}
	}

	// Failure by Endpoint Analysis
	if len(st.metrics.FailuresByEndpoint) > 0 {
		fmt.Println("\n🎯 Failures by Endpoint:")
		for endpoint, count := range st.metrics.FailuresByEndpoint {
			percentage := float64(count) / float64(totalFailed) * 100
			fmt.Printf("   %s: %d (%.1f%%)\n", endpoint, count, percentage)
		}
	}

	// Recent Failure Examples
	if len(st.metrics.FailureDetails) > 0 {
		fmt.Println("\n📝 Recent Failure Examples:")

		// Show up to 5 most recent failures
		start := 0
		if len(st.metrics.FailureDetails) > 5 {
			start = len(st.metrics.FailureDetails) - 5
		}

		for i := start; i < len(st.metrics.FailureDetails); i++ {
			failure := st.metrics.FailureDetails[i]
			fmt.Printf("   [%s] Request #%d: %s (%s) - %dms\n",
				failure.Timestamp.Format("15:04:05"),
				failure.RequestID,
				failure.ErrorMessage,
				failure.ErrorType,
				failure.Duration)
		}
	}

	// Failure Rate Trend Analysis
	st.generateFailureTrendAnalysis()
}

// generateFailureTrendAnalysis analyzes failure trends over time
func (st *StressTest) generateFailureTrendAnalysis() {
	if len(st.metrics.TimeSeries) < 2 {
		return
	}

	fmt.Println("\n📈 Failure Rate Trend:")

	// Show failure rate progression
	for i, point := range st.metrics.TimeSeries {
		if i == 0 || i == len(st.metrics.TimeSeries)-1 || i%3 == 0 {
			var failureRate float64
			if point.TotalRequests > 0 {
				failureRate = float64(point.FailedRequests) / float64(point.TotalRequests) * 100
			}

			elapsed := point.Timestamp.Sub(st.metrics.startTime)
			fmt.Printf("   %v: %.1f%% failure rate (%.1f RPS)\n",
				elapsed.Round(time.Second),
				failureRate,
				point.RequestsPerSec)
		}
	}
}

// exportDetailedStats exports detailed statistics to JSON file
func (st *StressTest) exportDetailedStats(duration time.Duration) {
	st.metrics.mutex.RLock()
	st.bugResults.mutex.RLock()
	defer st.metrics.mutex.RUnlock()
	defer st.bugResults.mutex.RUnlock()

	// Create comprehensive stats structure
	stats := map[string]interface{}{
		"test_config": map[string]interface{}{
			"duration":        duration.String(),
			"max_concurrency": st.config.MaxConcurrency,
			"target_url":      st.baseURL,
			"https_enabled":   st.config.UseHTTPS,
			"bug_discovery": map[string]interface{}{
				"enabled":              st.config.BugDiscovery.Enabled,
				"id_validation_tests":  st.config.BugDiscovery.IDValidationTests,
				"access_limit_tests":   st.config.BugDiscovery.AccessLimitTests,
				"expiration_tests":     st.config.BugDiscovery.ExpirationTests,
				"concurrent_tests":     st.config.BugDiscovery.ConcurrentLimitTests,
				"data_integrity_tests": st.config.BugDiscovery.DataIntegrityTests,
				"rapid_cycle_tests":    st.config.BugDiscovery.RapidCycleTests,
			},
		},
		"summary": map[string]interface{}{
			"total_requests":      atomic.LoadInt64(&st.metrics.TotalRequests),
			"successful_requests": atomic.LoadInt64(&st.metrics.SuccessfulRequests),
			"failed_requests":     atomic.LoadInt64(&st.metrics.FailedRequests),
			"requests_per_second": float64(atomic.LoadInt64(&st.metrics.TotalRequests)) / duration.Seconds(),
			"success_rate":        float64(atomic.LoadInt64(&st.metrics.SuccessfulRequests)) / float64(atomic.LoadInt64(&st.metrics.TotalRequests)) * 100,
			"max_concurrent":      atomic.LoadInt64(&st.metrics.MaxConcurrentReqs),
		},
		"response_times": map[string]interface{}{
			"average_ms": func() float64 {
				total := atomic.LoadInt64(&st.metrics.TotalRequests)
				if total == 0 {
					return 0
				}
				return float64(atomic.LoadInt64(&st.metrics.TotalResponseTime)) / float64(total)
			}(),
			"minimum_ms": st.metrics.MinResponseTime,
			"maximum_ms": st.metrics.MaxResponseTime,
		},
		"failure_analysis": map[string]interface{}{
			"by_type":     st.metrics.FailuresByType,
			"by_code":     st.metrics.FailuresByCode,
			"by_endpoint": st.metrics.FailuresByEndpoint,
			"details":     st.metrics.FailureDetails,
		},
		"bug_discovery": map[string]interface{}{
			"bugs_found": len(st.bugResults.Bugs),
			"bugs":       st.bugResults.Bugs,
		},
		"time_series": st.metrics.TimeSeries,
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	// Generate filename with timestamp
	filename := fmt.Sprintf("stress-test-results-%s.json", time.Now().Format("20060102-150405"))

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		fmt.Printf("⚠️  Failed to marshal stats to JSON: %v\n", err)
		return
	}

	// Write to file
	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		fmt.Printf("⚠️  Failed to write stats file: %v\n", err)
		return
	}

	fmt.Printf("\n💾 Detailed statistics exported to: %s\n", filename)

	// Also create a CSV summary for easy analysis
	st.exportCSVSummary(duration)
}

// exportCSVSummary exports a CSV summary of key metrics
func (st *StressTest) exportCSVSummary(duration time.Duration) {
	filename := fmt.Sprintf("stress-test-summary-%s.csv", time.Now().Format("20060102-150405"))

	csvContent := "timestamp,total_requests,successful_requests,failed_requests,rps,success_rate,avg_response_time_ms,max_concurrent\n"

	totalReqs := atomic.LoadInt64(&st.metrics.TotalRequests)
	successReqs := atomic.LoadInt64(&st.metrics.SuccessfulRequests)
	failedReqs := atomic.LoadInt64(&st.metrics.FailedRequests)
	rps := float64(totalReqs) / duration.Seconds()
	successRate := float64(successReqs) / float64(totalReqs) * 100

	var avgResponseTime float64
	if totalReqs > 0 {
		avgResponseTime = float64(atomic.LoadInt64(&st.metrics.TotalResponseTime)) / float64(totalReqs)
	}

	csvContent += fmt.Sprintf("%s,%d,%d,%d,%.2f,%.2f,%.2f,%d\n",
		time.Now().Format(time.RFC3339),
		totalReqs,
		successReqs,
		failedReqs,
		rps,
		successRate,
		avgResponseTime,
		atomic.LoadInt64(&st.metrics.MaxConcurrentReqs))

	if err := os.WriteFile(filename, []byte(csvContent), 0644); err != nil {
		fmt.Printf("⚠️  Failed to write CSV summary: %v\n", err)
		return
	}

	fmt.Printf("📊 CSV summary exported to: %s\n", filename)
}