// Package main provides a simplified stress test for the Content Storage Server
// focused primarily on RPS (Requests Per Second) measurement.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
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

// FailureCategory represents different types of failures
type FailureCategory struct {
	Name        string `json:"name"`
	Count       int64  `json:"count"`
	Percentage  float64 `json:"percentage"`
	Examples    []FailureDetail `json:"examples,omitempty"`
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

	mutex              sync.RWMutex
	startTime          time.Time
}

// StressTest manages the stress testing process
type StressTest struct {
	config     *StressTestConfig
	metrics    *TestMetrics
	httpClient *http.Client
	serverCmd  *exec.Cmd
	ctx        context.Context
	cancel     context.CancelFunc
	baseURL    string
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
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()
}

// NewStressTest creates a new stress test instance
func NewStressTest(config *StressTestConfig) *StressTest {
	ctx, cancel := context.WithCancel(context.Background())

	// Configure HTTP transport with optional TLS settings
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
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
		startTime:          time.Now(),
	}

	st := &StressTest{
		config: config,
		metrics: metrics,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		ctx:     ctx,
		cancel:  cancel,
		baseURL: fmt.Sprintf("%s://%s:%s", protocol, config.ServerHost, config.ServerPort),
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

// executeRequest executes a single API request
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

	// Simple POST request to create content
	statusCode, err := st.executePostContent(requestID)

	duration := time.Since(startTime)
	st.recordMetrics(requestID, duration, statusCode, err, "/api/v1/content/")
}

// executePostContent performs a POST request to store content
func (st *StressTest) executePostContent(requestID int64) (int, error) {
	// Generate simple content
	contentID := fmt.Sprintf("stress-test-%d", requestID)
	content := map[string]interface{}{
		"id":   contentID,
		"data": fmt.Sprintf("test data %d", requestID),
		"type": "text/plain",
	}

	jsonData, err := json.Marshal(content)
	if err != nil {
		return 0, err
	}

	resp, err := st.httpClient.Post(
		st.baseURL+"/api/v1/content/",
		"application/json",
		strings.NewReader(string(jsonData)),
	)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Read response body to ensure complete processing
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
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

	rps := float64(totalReqs) / duration.Seconds()
	successRate := float64(successReqs) / float64(totalReqs) * 100

	// Basic Performance Metrics
	fmt.Println("\n📈 PERFORMANCE METRICS")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("🚀 Requests Per Second:    %.1f RPS\n", rps)
	fmt.Printf("⏱️  Test Duration:          %v\n", duration.Round(time.Millisecond))
	fmt.Printf("📊 Total Requests:         %d\n", totalReqs)
	fmt.Printf("✅ Successful Requests:    %d (%.1f%%)\n", successReqs, successRate)
	fmt.Printf("❌ Failed Requests:        %d (%.1f%%)\n", failedReqs, 100-successRate)
	fmt.Printf("🔄 Max Concurrent:         %d\n", maxConcurrent)

	// Response Time Statistics
	st.generateResponseTimeReport()

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
	defer st.metrics.mutex.RUnlock()

	// Create comprehensive stats structure
	stats := map[string]interface{}{
		"test_config": map[string]interface{}{
			"duration":        duration.String(),
			"max_concurrency": st.config.MaxConcurrency,
			"target_url":      st.baseURL,
			"https_enabled":   st.config.UseHTTPS,
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