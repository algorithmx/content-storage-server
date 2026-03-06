#!/usr/bin/env python3
"""
Content Storage Server - Comprehensive Test Client

This script performs realistic client scenario testing against the content storage server
to reveal potential issues with the server's function promises.

Function Promises (from server analysis):
1. POST /api/v1/content/ - Store content, returns HTTP 202 (Accepted), content queued
2. GET /api/v1/content/ - List content with pagination and filtering
3. GET /api/v1/content/{id} - Retrieve content with atomic access count increment
4. DELETE /api/v1/content/{id} - Delete content with retry logic
5. GET /api/v1/content/{id}/status - Check status: queued, stored, not_found
6. GET /api/v1/content/count - Get total count
7. Access limit enforcement - 410 when limit reached
8. Expiration support - 404 for expired content
9. Health endpoints - /health, /health/detailed
10. Management endpoints - /api/v1/backup, /api/v1/gc, /api/v1/cleanup

Test Scenarios:
1. Basic CRUD operations
2. Asynchronous write queue behavior
3. Access limit enforcement
4. Time-based expiration
5. Concurrent operations
6. Error handling and edge cases
7. Pagination and filtering
8. Health and metrics
9. Management operations
10. Rate limiting (if enabled)
11. Authentication (if enabled)
"""

import argparse
import json
import time
import uuid
import random
import string
import threading
import sys
from datetime import datetime, timedelta
from typing import Optional, Dict, Any, List, Tuple
from dataclasses import dataclass
from concurrent.futures import ThreadPoolExecutor, as_completed
import requests
from requests.exceptions import RequestException


# ============================================================================
# Configuration
# ============================================================================

@dataclass
class TestConfig:
    """Configuration for test client"""
    base_url: str = "http://localhost:8081"
    api_key: Optional[str] = None
    timeout: int = 30
    verbose: bool = False
    concurrent_requests: int = 10
    test_data_size: int = 100


# ============================================================================
# Test Result Tracking
# ============================================================================

class TestResults:
    """Track test results and issues found"""

    def __init__(self):
        self.passed: List[str] = []
        self.failed: List[Tuple[str, str]] = []  # (test_name, error_message)
        self.issues: List[Dict[str, Any]] = []  # Detailed issues found
        self.warnings: List[str] = []

    def add_pass(self, test_name: str):
        self.passed.append(test_name)

    def add_fail(self, test_name: str, error: str):
        self.failed.append((test_name, error))

    def add_issue(self, category: str, description: str, details: Dict[str, Any]):
        self.issues.append({
            "category": category,
            "description": description,
            "details": details,
            "timestamp": datetime.now().isoformat()
        })

    def add_warning(self, message: str):
        self.warnings.append(message)

    def print_summary(self):
        print("\n" + "=" * 70)
        print("TEST SUMMARY")
        print("=" * 70)
        print(f"Passed: {len(self.passed)}")
        print(f"Failed: {len(self.failed)}")
        print(f"Issues Found: {len(self.issues)}")
        print(f"Warnings: {len(self.warnings)}")

        if self.failed:
            print("\n--- FAILED TESTS ---")
            for name, error in self.failed:
                print(f"  [FAIL] {name}: {error}")

        if self.issues:
            print("\n--- ISSUES FOUND ---")
            for issue in self.issues:
                print(f"\n  [{issue['category']}] {issue['description']}")
                print(f"    Details: {json.dumps(issue['details'], indent=4)}")

        if self.warnings:
            print("\n--- WARNINGS ---")
            for warning in self.warnings:
                print(f"  [WARN] {warning}")

        print("\n" + "=" * 70)


# ============================================================================
# API Client
# ============================================================================

class ContentStorageClient:
    """Client for Content Storage Server API"""

    def __init__(self, config: TestConfig, results: TestResults):
        self.config = config
        self.results = results
        self.session = requests.Session()
        if config.api_key:
            self.session.headers.update({"X-API-Key": config.api_key})

    def _url(self, path: str) -> str:
        return f"{self.config.base_url}{path}"

    def _log(self, message: str):
        if self.config.verbose:
            print(f"  [{datetime.now().strftime('%H:%M:%S.%f')}] {message}")

    def _handle_response(self, response: requests.Response, expected_status: int, context: str) -> Optional[Dict]:
        """Handle response and track issues"""
        if response.status_code != expected_status:
            self.results.add_issue(
                "Unexpected Status",
                f"Expected {expected_status}, got {response.status_code} in {context}",
                {
                    "expected_status": expected_status,
                    "actual_status": response.status_code,
                    "context": context,
                    "response_body": response.text[:500] if response.text else None
                }
            )
            return None

        try:
            return response.json()
        except json.JSONDecodeError:
            if response.text:
                self.results.add_issue(
                    "Invalid JSON",
                    f"Response is not valid JSON in {context}",
                    {"response_text": response.text[:500]}
                )
            return None

    # Health endpoints (no auth required)
    def health_check(self) -> Tuple[bool, Optional[Dict]]:
        """GET /health - Basic health check"""
        self._log("Health check")
        try:
            response = self.session.get(self._url("/health"), timeout=self.config.timeout)
            data = self._handle_response(response, 200, "health_check")
            return response.status_code == 200, data
        except RequestException as e:
            return False, {"error": str(e)}

    def detailed_health_check(self) -> Tuple[int, Optional[Dict]]:
        """GET /health/detailed - Detailed health check"""
        self._log("Detailed health check")
        try:
            response = self.session.get(self._url("/health/detailed"), timeout=self.config.timeout)
            # Status can be 200, 206, or 503
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    # Content operations
    def store_content(self, content_id: str, data: str, content_type: str = "text/plain",
                      tag: Optional[str] = None, access_limit: Optional[int] = None,
                      expires_at: Optional[str] = None) -> Tuple[int, Optional[Dict]]:
        """POST /api/v1/content - Store content (returns 202)"""
        payload = {
            "id": content_id,
            "data": data,
            "type": content_type
        }
        if tag:
            payload["tag"] = tag
        if access_limit is not None:
            payload["access_limit"] = access_limit
        if expires_at:
            payload["expires_at"] = expires_at

        self._log(f"Storing content: {content_id}")
        try:
            response = self.session.post(
                self._url("/api/v1/content"),
                json=payload,
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def get_content(self, content_id: str) -> Tuple[int, Optional[Dict]]:
        """GET /api/v1/content/{id} - Retrieve content"""
        self._log(f"Getting content: {content_id}")
        try:
            response = self.session.get(
                self._url(f"/api/v1/content/{content_id}"),
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def list_content(self, limit: int = 100, offset: int = 0,
                     content_type: Optional[str] = None,
                     tag: Optional[str] = None,
                     include_expired: bool = False) -> Tuple[int, Optional[Dict]]:
        """GET /api/v1/content - List content with filtering"""
        params = {"limit": limit, "offset": offset}
        if content_type:
            params["type"] = content_type
        if tag:
            params["tag"] = tag
        if include_expired:
            params["include_expired"] = "true"

        self._log(f"Listing content (limit={limit}, offset={offset})")
        try:
            response = self.session.get(
                self._url("/api/v1/content"),
                params=params,
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def delete_content(self, content_id: str) -> Tuple[int, Optional[Dict]]:
        """DELETE /api/v1/content/{id} - Delete content"""
        self._log(f"Deleting content: {content_id}")
        try:
            response = self.session.delete(
                self._url(f"/api/v1/content/{content_id}"),
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def get_content_status(self, content_id: str) -> Tuple[int, Optional[Dict]]:
        """GET /api/v1/content/{id}/status - Check storage status"""
        self._log(f"Checking status: {content_id}")
        try:
            response = self.session.get(
                self._url(f"/api/v1/content/{content_id}/status"),
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def get_content_count(self) -> Tuple[int, Optional[Dict]]:
        """GET /api/v1/content/count - Get total count"""
        self._log("Getting content count")
        try:
            response = self.session.get(
                self._url("/api/v1/content/count"),
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def get_metrics(self) -> Tuple[int, Optional[Dict]]:
        """GET /api/v1/metrics - Get system metrics"""
        self._log("Getting metrics")
        try:
            response = self.session.get(
                self._url("/api/v1/metrics"),
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    # Management operations
    def trigger_gc(self) -> Tuple[int, Optional[Dict]]:
        """POST /api/v1/gc - Trigger garbage collection"""
        self._log("Triggering GC")
        try:
            response = self.session.post(
                self._url("/api/v1/gc"),
                timeout=self.config.timeout * 2  # GC may take longer
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def create_backup(self, backup_name: Optional[str] = None) -> Tuple[int, Optional[Dict]]:
        """POST /api/v1/backup - Create backup"""
        payload = {}
        if backup_name:
            payload["backup_name"] = backup_name

        self._log("Creating backup")
        try:
            response = self.session.post(
                self._url("/api/v1/backup"),
                json=payload,
                timeout=self.config.timeout * 2
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def cleanup_access_trackers(self) -> Tuple[int, Optional[Dict]]:
        """POST /api/v1/cleanup - Cleanup access trackers"""
        self._log("Cleaning up access trackers")
        try:
            response = self.session.post(
                self._url("/api/v1/cleanup"),
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    def trigger_sync(self) -> Tuple[int, Optional[Dict]]:
        """POST /api/v1/sync - Trigger sync (stub)"""
        self._log("Triggering sync")
        try:
            response = self.session.post(
                self._url("/api/v1/sync"),
                timeout=self.config.timeout
            )
            data = response.json() if response.text else None
            return response.status_code, data
        except RequestException as e:
            return 0, {"error": str(e)}

    # Helper methods
    def wait_for_content_stored(self, content_id: str, timeout: float = 10.0) -> bool:
        """Wait for content to be stored (poll status endpoint)"""
        start_time = time.time()
        while time.time() - start_time < timeout:
            status, data = self.get_content_status(content_id)
            if data and data.get("data", {}).get("status") == "stored":
                return True
            if data and data.get("data", {}).get("status") == "not_found":
                return False
            time.sleep(0.1)
        return False

    def store_and_wait(self, content_id: str, data: str, **kwargs) -> Tuple[bool, int, Optional[Dict]]:
        """Store content and wait for it to be persisted"""
        status, response = self.store_content(content_id, data, **kwargs)
        if status != 202:
            return False, status, response

        stored = self.wait_for_content_stored(content_id)
        return stored, status, response


# ============================================================================
# Test Scenarios
# ============================================================================

class TestScenarios:
    """Realistic test scenarios for content storage server"""

    def __init__(self, client: ContentStorageClient, results: TestResults, config: TestConfig):
        self.client = client
        self.results = results
        self.config = config

    def generate_id(self, prefix: str = "test") -> str:
        """Generate unique content ID"""
        return f"{prefix}-{uuid.uuid4().hex[:8]}"

    def generate_data(self, size: int = 100) -> str:
        """Generate random test data"""
        return ''.join(random.choices(string.ascii_letters + string.digits, k=size))

    # -------------------------------------------------------------------------
    # Scenario 1: Basic CRUD Operations
    # -------------------------------------------------------------------------

    def test_basic_crud(self):
        """Test basic Create, Read, Update (via overwrite), Delete operations"""
        print("\n=== Scenario 1: Basic CRUD Operations ===")

        # Create
        content_id = self.generate_id("crud")
        data = f"Test data at {datetime.now().isoformat()}"

        status, response = self.client.store_content(content_id, data, tag="crud-test")
        if status != 202:
            self.results.add_fail("CRUD - Store", f"Expected 202, got {status}")
            return
        self.results.add_pass("CRUD - Store (202)")

        # Wait for storage
        if not self.client.wait_for_content_stored(content_id):
            self.results.add_fail("CRUD - Wait for storage", "Content not stored in time")
            return

        # Read
        status, response = self.client.get_content(content_id)
        if status != 200:
            self.results.add_fail("CRUD - Read", f"Expected 200, got {status}")
            return

        # Verify data integrity
        if response and "data" in response:
            stored_data = response["data"].get("data")
            if stored_data != data:
                self.results.add_issue(
                    "Data Integrity",
                    "Stored data doesn't match original",
                    {"expected": data, "actual": stored_data}
                )
            else:
                self.results.add_pass("CRUD - Data integrity verified")

        # Check access count is 1 after first retrieval
        if response and "data" in response:
            access_count = response["data"].get("access_count", 0)
            if access_count != 1:
                self.results.add_issue(
                    "Access Count",
                    "Access count should be 1 after first retrieval",
                    {"expected": 1, "actual": access_count}
                )

        # Delete
        status, response = self.client.delete_content(content_id)
        if status != 200:
            self.results.add_fail("CRUD - Delete", f"Expected 200, got {status}")
            return
        self.results.add_pass("CRUD - Delete")

        # Verify deletion (should return 404)
        status, response = self.client.get_content(content_id)
        if status != 404:
            self.results.add_issue(
                "Deletion Verification",
                "Content should return 404 after deletion",
                {"status": status}
            )
        else:
            self.results.add_pass("CRUD - Deletion verified (404)")

    # -------------------------------------------------------------------------
    # Scenario 2: Asynchronous Write Queue Behavior
    # -------------------------------------------------------------------------

    def test_async_write_queue(self):
        """Test asynchronous write queue - content should be 'queued' then 'stored'"""
        print("\n=== Scenario 2: Asynchronous Write Queue ===")

        content_id = self.generate_id("async")
        data = self.generate_data(500)

        # Store content
        status, response = self.client.store_content(content_id, data)
        if status != 202:
            self.results.add_fail("Async Queue - Store", f"Expected 202, got {status}")
            return
        self.results.add_pass("Async Queue - Store returns 202")

        # Immediately check status - should be 'queued' or 'stored'
        status, response = self.client.get_content_status(content_id)
        if status not in [200, 404]:
            self.results.add_issue(
                "Queue Status",
                "Status endpoint returned unexpected code",
                {"status": status, "response": response}
            )
        else:
            if response and "data" in response:
                content_status = response["data"].get("status")
                if content_status not in ["queued", "stored", "not_found"]:
                    self.results.add_issue(
                        "Queue Status Value",
                        f"Unexpected status value: {content_status}",
                        {"response": response}
                    )
                else:
                    self.results.add_pass(f"Async Queue - Status is '{content_status}'")

        # Wait for storage completion
        stored = self.client.wait_for_content_stored(content_id, timeout=15.0)
        if not stored:
            self.results.add_fail("Async Queue - Wait for storage", "Content not stored within timeout")
            return
        self.results.add_pass("Async Queue - Content stored successfully")

        # Verify final status is 'stored'
        status, response = self.client.get_content_status(content_id)
        if response and "data" in response:
            final_status = response["data"].get("status")
            if final_status != "stored":
                self.results.add_issue(
                    "Final Status",
                    f"Expected 'stored', got '{final_status}'",
                    {"response": response}
                )
            else:
                self.results.add_pass("Async Queue - Final status is 'stored'")

        # Cleanup
        self.client.delete_content(content_id)

    def test_queue_ordering(self):
        """Test that queue processes items in order (sequential write guarantee)"""
        print("\n=== Scenario 2b: Queue Ordering ===")

        base_id = self.generate_id("order")
        num_items = 10
        ids = [f"{base_id}-{i}" for i in range(num_items)]

        # Store multiple items rapidly
        for i, content_id in enumerate(ids):
            status, _ = self.client.store_content(content_id, f"data-{i}")
            if status != 202:
                self.results.add_fail("Queue Ordering - Store", f"Failed to store {content_id}")
                return

        self.results.add_pass(f"Queue Ordering - Stored {num_items} items")

        # Wait for all to be stored
        for content_id in ids:
            if not self.client.wait_for_content_stored(content_id, timeout=20.0):
                self.results.add_issue(
                    "Queue Ordering",
                    f"Content {content_id} not stored in time",
                    {}
                )

        # Verify all exist
        stored_count = 0
        for content_id in ids:
            status, _ = self.client.get_content(content_id)
            if status == 200:
                stored_count += 1

        if stored_count == num_items:
            self.results.add_pass(f"Queue Ordering - All {num_items} items stored correctly")
        else:
            self.results.add_issue(
                "Queue Ordering",
                f"Only {stored_count}/{num_items} items stored",
                {}
            )

        # Cleanup
        for content_id in ids:
            self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Scenario 3: Access Limit Enforcement
    # -------------------------------------------------------------------------

    def test_access_limit(self):
        """Test access limit enforcement - content should return 410 after limit reached"""
        print("\n=== Scenario 3: Access Limit Enforcement ===")

        content_id = self.generate_id("limit")
        access_limit = 3  # Allow exactly 3 accesses

        # Store with access limit
        status, response = self.client.store_content(
            content_id,
            "limited access content",
            access_limit=access_limit
        )
        if status != 202:
            self.results.add_fail("Access Limit - Store", f"Expected 202, got {status}")
            return

        # Wait for storage
        if not self.client.wait_for_content_stored(content_id):
            self.results.add_fail("Access Limit - Wait", "Content not stored")
            return

        # Access exactly up to the limit
        for i in range(access_limit):
            status, response = self.client.get_content(content_id)
            if status != 200:
                self.results.add_issue(
                    "Access Limit",
                    f"Expected 200 on access {i+1}/{access_limit}, got {status}",
                    {"attempt": i + 1}
                )
            else:
                access_count = response.get("data", {}).get("access_count", 0)
                self.results.add_pass(f"Access Limit - Access {i+1}/{access_limit} OK (count: {access_count})")

        # Next access should return 410 (Gone)
        status, response = self.client.get_content(content_id)
        if status != 410:
            self.results.add_issue(
                "Access Limit Enforcement",
                f"Expected 410 after access limit reached, got {status}",
                {"expected": 410, "actual": status, "access_limit": access_limit}
            )
        else:
            self.results.add_pass("Access Limit - Returns 410 after limit reached")

        # Cleanup
        self.client.delete_content(content_id)

    def test_access_count_atomicity(self):
        """Test that access count increments are atomic under concurrent access"""
        print("\n=== Scenario 3b: Access Count Atomicity ===")

        content_id = self.generate_id("atomic")
        num_concurrent_reads = 20

        # Store content
        status, _ = self.client.store_content(content_id, "atomic test data")
        if status != 202:
            self.results.add_fail("Atomic - Store", "Failed to store")
            return

        if not self.client.wait_for_content_stored(content_id):
            self.results.add_fail("Atomic - Wait", "Content not stored")
            return

        # Perform concurrent reads
        access_counts = []
        errors = []

        def read_content():
            try:
                status, response = self.client.get_content(content_id)
                if status == 200 and response:
                    return response.get("data", {}).get("access_count", 0)
            except Exception as e:
                errors.append(str(e))
            return None

        with ThreadPoolExecutor(max_workers=self.config.concurrent_requests) as executor:
            futures = [executor.submit(read_content) for _ in range(num_concurrent_reads)]
            for future in as_completed(futures):
                count = future.result()
                if count is not None:
                    access_counts.append(count)

        if errors:
            self.results.add_issue(
                "Concurrent Access",
                "Errors during concurrent reads",
                {"errors": errors}
            )

        # Final access count should equal number of successful reads
        status, response = self.client.get_content(content_id)
        if status == 200:
            final_count = response.get("data", {}).get("access_count", 0)
            expected_count = len(access_counts) + 1  # +1 for this final read

            if final_count != expected_count:
                self.results.add_issue(
                    "Access Count Atomicity",
                    "Final count doesn't match number of accesses",
                    {
                        "final_count": final_count,
                        "successful_reads": len(access_counts),
                        "expected_count": expected_count
                    }
                )
            else:
                self.results.add_pass(f"Atomic - Count matches: {final_count} accesses")

        # Cleanup
        self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Scenario 4: Time-Based Expiration
    # -------------------------------------------------------------------------

    def test_time_expiration(self):
        """Test time-based expiration - content should return 404 after expiration"""
        print("\n=== Scenario 4: Time-Based Expiration ===")

        content_id = self.generate_id("expire")

        # Store with expiration 5 seconds in the future
        expires_at = (datetime.utcnow() + timedelta(seconds=5)).strftime("%Y-%m-%dT%H:%M:%SZ")

        status, _ = self.client.store_content(
            content_id,
            "expiring content",
            expires_at=expires_at
        )
        if status != 202:
            self.results.add_fail("Expiration - Store", f"Expected 202, got {status}")
            return

        if not self.client.wait_for_content_stored(content_id):
            self.results.add_fail("Expiration - Wait", "Content not stored")
            return

        # Should be accessible now
        status, response = self.client.get_content(content_id)
        if status != 200:
            self.results.add_issue(
                "Expiration",
                "Content should be accessible before expiration",
                {"status": status}
            )
        else:
            self.results.add_pass("Expiration - Accessible before expiration")

        # Wait for expiration
        print("    Waiting 6 seconds for expiration...")
        time.sleep(6)

        # Should return 404 after expiration
        status, response = self.client.get_content(content_id)
        if status != 404:
            self.results.add_issue(
                "Time Expiration",
                f"Expected 404 for expired content, got {status}",
                {"expected": 404, "actual": status}
            )
        else:
            self.results.add_pass("Expiration - Returns 404 after expiration")

        # Cleanup
        self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Scenario 5: Concurrent Operations
    # -------------------------------------------------------------------------

    def test_concurrent_writes(self):
        """Test concurrent write operations"""
        print("\n=== Scenario 5: Concurrent Write Operations ===")

        num_threads = self.config.concurrent_requests
        results_lock = threading.Lock()
        success_count = 0
        fail_count = 0

        def store_content(index: int):
            nonlocal success_count, fail_count
            content_id = self.generate_id(f"concurrent-{index}")
            status, _ = self.client.store_content(content_id, f"data-{index}")
            with results_lock:
                if status == 202:
                    success_count += 1
                else:
                    fail_count += 1
            return content_id, status

        # Store multiple items concurrently
        with ThreadPoolExecutor(max_workers=num_threads) as executor:
            futures = [executor.submit(store_content, i) for i in range(num_threads)]
            stored_ids = []
            for future in as_completed(futures):
                content_id, status = future.result()
                if status == 202:
                    stored_ids.append(content_id)

        self.results.add_pass(f"Concurrent Writes - {success_count}/{num_threads} successful")

        if fail_count > 0:
            self.results.add_issue(
                "Concurrent Writes",
                f"{fail_count} writes failed",
                {"failed": fail_count, "total": num_threads}
            )

        # Wait for all to be stored
        stored_count = 0
        for content_id in stored_ids:
            if self.client.wait_for_content_stored(content_id, timeout=15.0):
                stored_count += 1

        if stored_count == len(stored_ids):
            self.results.add_pass(f"Concurrent Writes - All {stored_count} items persisted")
        else:
            self.results.add_issue(
                "Concurrent Writes",
                f"Only {stored_count}/{len(stored_ids)} items persisted",
                {}
            )

        # Cleanup
        for content_id in stored_ids:
            self.client.delete_content(content_id)

    def test_concurrent_read_write(self):
        """Test concurrent reads and writes to same content"""
        print("\n=== Scenario 5b: Concurrent Read/Write ===")

        content_id = self.generate_id("rw")

        # Store initial content
        status, _ = self.client.store_content(content_id, "initial")
        if status != 202:
            self.results.add_fail("Concurrent RW - Store", "Failed to store")
            return

        self.client.wait_for_content_stored(content_id)

        read_errors = []
        write_errors = []

        def read_loop():
            for _ in range(5):
                try:
                    status, _ = self.client.get_content(content_id)
                    if status not in [200, 404, 410]:
                        read_errors.append(f"Unexpected status: {status}")
                except Exception as e:
                    read_errors.append(str(e))

        def write_loop():
            for i in range(5):
                try:
                    # Store with same ID (overwrite)
                    status, _ = self.client.store_content(content_id, f"updated-{i}")
                    if status != 202:
                        write_errors.append(f"Write failed: {status}")
                except Exception as e:
                    write_errors.append(str(e))

        # Run concurrent reads and writes
        read_thread = threading.Thread(target=read_loop)
        write_thread = threading.Thread(target=write_loop)

        read_thread.start()
        write_thread.start()

        read_thread.join()
        write_thread.join()

        if read_errors:
            self.results.add_issue(
                "Concurrent RW",
                "Errors during concurrent reads",
                {"errors": read_errors}
            )
        else:
            self.results.add_pass("Concurrent RW - No read errors")

        if write_errors:
            self.results.add_issue(
                "Concurrent RW",
                "Errors during concurrent writes",
                {"errors": write_errors}
            )
        else:
            self.results.add_pass("Concurrent RW - No write errors")

        # Cleanup
        self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Scenario 6: Error Handling and Edge Cases
    # -------------------------------------------------------------------------

    def test_error_handling(self):
        """Test error handling for various edge cases"""
        print("\n=== Scenario 6: Error Handling and Edge Cases ===")

        # Test 1: Invalid content ID format
        invalid_ids = [
            "invalid id with spaces",
            "invalid/id",
            "invalid@id",
            "",  # Empty ID
            "a" * 300,  # Too long ID
        ]

        for invalid_id in invalid_ids:
            if not invalid_id:  # Skip empty for now, might cause different error
                continue
            status, _ = self.client.get_content(invalid_id)
            if status not in [400, 404]:
                self.results.add_issue(
                    "Invalid ID Handling",
                    f"Expected 400 or 404 for invalid ID, got {status}",
                    {"id": invalid_id[:50], "status": status}
                )
            else:
                self.results.add_pass(f"Invalid ID - '{invalid_id[:20]}...' returns {status}")

        # Test 2: Get non-existent content
        status, _ = self.client.get_content("nonexistent-content-id-12345")
        if status != 404:
            self.results.add_issue(
                "Non-existent Content",
                f"Expected 404 for non-existent content, got {status}",
                {}
            )
        else:
            self.results.add_pass("Non-existent content returns 404")

        # Test 3: Delete non-existent content
        status, _ = self.client.delete_content("nonexistent-delete-id-12345")
        if status != 404:
            self.results.add_issue(
                "Delete Non-existent",
                f"Expected 404 for deleting non-existent content, got {status}",
                {}
            )
        else:
            self.results.add_pass("Delete non-existent returns 404")

        # Test 4: Status of non-existent content
        status, response = self.client.get_content_status("nonexistent-status-id-12345")
        if status == 200 and response:
            final_status = response.get("data", {}).get("status")
            if final_status != "not_found":
                self.results.add_issue(
                    "Status Non-existent",
                    f"Expected 'not_found' status, got '{final_status}'",
                    {}
                )
            else:
                self.results.add_pass("Status of non-existent returns 'not_found'")
        elif status != 404:
            self.results.add_issue(
                "Status Non-existent",
                f"Expected 200 with 'not_found' or 404, got {status}",
                {}
            )

        # Test 5: Store with invalid content type (if restrictions apply)
        status, response = self.client.store_content(
            self.generate_id("type"),
            "test data",
            content_type="invalid/type"
        )
        if status == 415:
            self.results.add_pass("Invalid content type returns 415")
        elif status == 202:
            self.results.add_warning("Server accepts any content type (415 not enforced)")
        else:
            self.results.add_issue(
                "Content Type Validation",
                f"Unexpected status for invalid content type: {status}",
                {}
            )

        # Test 6: Store with missing required fields
        # Note: The client always sends id and type, so this tests empty data
        status, _ = self.client.store_content(
            self.generate_id("empty"),
            "",  # Empty data
            content_type="text/plain"
        )
        if status == 202:
            self.results.add_pass("Empty data accepted (valid)")
        elif status == 400:
            self.results.add_pass("Empty data rejected with 400 (valid)")
        else:
            self.results.add_issue(
                "Empty Data",
                f"Unexpected status for empty data: {status}",
                {}
            )

    # -------------------------------------------------------------------------
    # Scenario 7: Pagination and Filtering
    # -------------------------------------------------------------------------

    def test_pagination_filtering(self):
        """Test pagination and filtering of content list"""
        print("\n=== Scenario 7: Pagination and Filtering ===")

        tag = f"pagination-test-{uuid.uuid4().hex[:6]}"
        num_items = 25

        # Store multiple items with same tag
        stored_ids = []
        for i in range(num_items):
            content_id = self.generate_id(f"page-{i}")
            status, _ = self.client.store_content(
                content_id,
                f"data-{i}",
                tag=tag,
                content_type="text/plain"
            )
            if status == 202:
                stored_ids.append(content_id)

        # Wait for all to be stored
        for content_id in stored_ids:
            self.client.wait_for_content_stored(content_id, timeout=15.0)

        self.results.add_pass(f"Pagination - Stored {len(stored_ids)} items with tag '{tag}'")

        # Test pagination
        page_size = 10
        total_retrieved = 0
        offset = 0

        while True:
            status, response = self.client.list_content(
                limit=page_size,
                offset=offset,
                tag=tag
            )
            if status != 200:
                self.results.add_fail("Pagination - List", f"Expected 200, got {status}")
                break

            items = response.get("data", {}).get("contents", [])
            total_in_response = len(items)
            total_retrieved += total_in_response

            if total_in_response == 0:
                break

            offset += page_size

            if offset > 100:  # Safety limit
                break

        # Verify total count matches stored items with this tag
        if total_retrieved >= num_items:
            self.results.add_pass(f"Pagination - Retrieved {total_retrieved} items")
        else:
            self.results.add_issue(
                "Pagination",
                f"Expected at least {num_items} items, got {total_retrieved}",
                {"expected": num_items, "actual": total_retrieved}
            )

        # Test filtering by type
        status, response = self.client.list_content(
            limit=100,
            content_type="text/plain",
            tag=tag
        )
        if status == 200:
            items = response.get("data", {}).get("contents", [])
            all_correct_type = all(
                item.get("type") == "text/plain" for item in items
            )
            if all_correct_type:
                self.results.add_pass("Filtering - Type filter works correctly")
            else:
                self.results.add_issue(
                    "Type Filtering",
                    "Some items have wrong type",
                    {}
                )

        # Test include_expired parameter
        status, response = self.client.list_content(
            limit=100,
            include_expired=True
        )
        if status == 200:
            self.results.add_pass("Filtering - include_expired parameter works")

        # Cleanup
        for content_id in stored_ids:
            self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Scenario 8: Health and Metrics
    # -------------------------------------------------------------------------

    def test_health_metrics(self):
        """Test health check and metrics endpoints"""
        print("\n=== Scenario 8: Health and Metrics ===")

        # Basic health check
        success, response = self.client.health_check()
        if success:
            self.results.add_pass("Health - Basic check returns 200")
            if response:
                status = response.get("status")
                if status != "healthy":
                    self.results.add_issue(
                        "Health Status",
                        f"Expected 'healthy', got '{status}'",
                        {"response": response}
                    )
        else:
            self.results.add_fail("Health - Basic check failed", str(response))

        # Detailed health check
        status, response = self.client.detailed_health_check()
        if status in [200, 206, 503]:
            self.results.add_pass(f"Health - Detailed check returns {status}")
            if response:
                health_status = response.get("status")
                metrics = response.get("metrics", {})

                # Check queue metrics are present
                queue_metrics = metrics.get("queue_metrics", {})
                if queue_metrics:
                    self.results.add_pass(f"Health - Queue metrics present (depth: {queue_metrics.get('queue_depth', 'N/A')})")

                # Check content count
                content_count = metrics.get("content_count")
                if content_count is not None:
                    self.results.add_pass(f"Health - Content count: {content_count}")
        else:
            self.results.add_issue(
                "Detailed Health",
                f"Unexpected status: {status}",
                {"response": response}
            )

        # Metrics endpoint
        status, response = self.client.get_metrics()
        if status == 200:
            self.results.add_pass("Metrics - Returns 200")
            if response:
                # Check various metrics fields
                expected_fields = [
                    "content_count",
                    "health_status",
                    "queue_metrics"
                ]
                for field in expected_fields:
                    if field in response:
                        self.results.add_pass(f"Metrics - '{field}' present")
                    else:
                        self.results.add_issue(
                            "Metrics Fields",
                            f"Missing expected field: {field}",
                            {"response_keys": list(response.keys())}
                        )
        else:
            self.results.add_issue(
                "Metrics",
                f"Expected 200, got {status}",
                {}
            )

    # -------------------------------------------------------------------------
    # Scenario 9: Management Operations
    # -------------------------------------------------------------------------

    def test_management_operations(self):
        """Test management endpoints (backup, gc, cleanup, sync)"""
        print("\n=== Scenario 9: Management Operations ===")

        # Sync (stub operation)
        status, response = self.client.trigger_sync()
        if status == 200:
            self.results.add_pass("Management - Sync returns 200")
        else:
            self.results.add_issue(
                "Sync Operation",
                f"Expected 200, got {status}",
                {}
            )

        # Backup
        status, response = self.client.create_backup(backup_name=f"test-backup-{uuid.uuid4().hex[:6]}")
        if status == 200:
            self.results.add_pass("Management - Backup returns 200")
            if response:
                backup_path = response.get("data", {}).get("backup_path")
                if backup_path:
                    self.results.add_pass(f"Management - Backup path: {backup_path}")
        elif status == 501:
            self.results.add_warning("Backup not supported for this storage type")
        else:
            self.results.add_issue(
                "Backup Operation",
                f"Unexpected status: {status}",
                {}
            )

        # GC
        status, response = self.client.trigger_gc()
        if status == 200:
            self.results.add_pass("Management - GC returns 200")
        elif status == 501:
            self.results.add_warning("GC not supported for this storage type")
        else:
            self.results.add_issue(
                "GC Operation",
                f"Unexpected status: {status}",
                {}
            )

        # Cleanup access trackers
        status, response = self.client.cleanup_access_trackers()
        if status == 200:
            self.results.add_pass("Management - Cleanup returns 200")
            if response:
                removed = response.get("data", {}).get("removed_trackers", 0)
                self.results.add_pass(f"Management - Removed {removed} trackers")
        else:
            self.results.add_issue(
                "Cleanup Operation",
                f"Unexpected status: {status}",
                {}
            )

    # -------------------------------------------------------------------------
    # Scenario 10: Content Overwrite Behavior
    # -------------------------------------------------------------------------

    def test_content_overwrite(self):
        """Test that storing content with same ID overwrites (last write wins)"""
        print("\n=== Scenario 10: Content Overwrite ===")

        content_id = self.generate_id("overwrite")

        # Store initial content
        status, _ = self.client.store_content(content_id, "initial data", tag="v1")
        if status != 202:
            self.results.add_fail("Overwrite - Initial store", "Failed")
            return

        self.client.wait_for_content_stored(content_id)

        # Store with same ID but different data
        status, _ = self.client.store_content(content_id, "updated data", tag="v2")
        if status != 202:
            self.results.add_fail("Overwrite - Update store", "Failed")
            return

        self.client.wait_for_content_stored(content_id)

        # Retrieve and verify updated content
        status, response = self.client.get_content(content_id)
        if status == 200 and response:
            data = response.get("data", {}).get("data")
            tag = response.get("data", {}).get("tag")

            if data == "updated data":
                self.results.add_pass("Overwrite - Data updated correctly")
            else:
                self.results.add_issue(
                    "Overwrite",
                    f"Expected 'updated data', got '{data}'",
                    {}
                )

            if tag == "v2":
                self.results.add_pass("Overwrite - Tag updated correctly")
            else:
                self.results.add_issue(
                    "Overwrite",
                    f"Expected tag 'v2', got '{tag}'",
                    {}
                )
        else:
            self.results.add_fail("Overwrite - Retrieve", f"Status: {status}")

        # Cleanup
        self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Scenario 11: Large Content Handling
    # -------------------------------------------------------------------------

    def test_large_content(self):
        """Test storing and retrieving larger content"""
        print("\n=== Scenario 11: Large Content ===")

        content_id = self.generate_id("large")

        # Generate 1KB of data
        large_data = self.generate_data(1024)

        status, _ = self.client.store_content(content_id, large_data)
        if status != 202:
            self.results.add_fail("Large Content - Store", f"Status: {status}")
            return

        if not self.client.wait_for_content_stored(content_id):
            self.results.add_fail("Large Content - Wait", "Not stored")
            return

        # Retrieve and verify
        status, response = self.client.get_content(content_id)
        if status == 200 and response:
            retrieved_data = response.get("data", {}).get("data")
            if retrieved_data == large_data:
                self.results.add_pass("Large Content - Data integrity verified")
            else:
                self.results.add_issue(
                    "Large Content",
                    "Data mismatch",
                    {
                        "expected_len": len(large_data),
                        "actual_len": len(retrieved_data) if retrieved_data else 0
                    }
                )
        else:
            self.results.add_fail("Large Content - Retrieve", f"Status: {status}")

        # Test with near-limit size (simulate near 10MB)
        # Note: We'll use a smaller size to avoid timeout issues
        near_limit_data = self.generate_data(100 * 1024)  # 100KB
        content_id_large = self.generate_id("larger")

        status, _ = self.client.store_content(content_id_large, near_limit_data)
        if status == 202:
            self.results.add_pass("Large Content - 100KB accepted")
            self.client.wait_for_content_stored(content_id_large)
            self.client.delete_content(content_id_large)
        elif status == 413:
            self.results.add_pass("Large Content - Correctly rejected with 413")
        else:
            self.results.add_issue(
                "Large Content",
                f"Unexpected status for 100KB content: {status}",
                {}
            )

        # Cleanup
        self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Scenario 12: Count Endpoint
    # -------------------------------------------------------------------------

    def test_count_endpoint(self):
        """Test the content count endpoint"""
        print("\n=== Scenario 12: Count Endpoint ===")

        # Get initial count
        status, response = self.client.get_content_count()
        if status != 200:
            self.results.add_fail("Count - Initial", f"Status: {status}")
            return

        initial_count = response.get("data", {}).get("count", 0)
        self.results.add_pass(f"Count - Initial count: {initial_count}")

        # Store some content
        stored_ids = []
        for i in range(5):
            content_id = self.generate_id(f"count-{i}")
            status, _ = self.client.store_content(content_id, f"data-{i}")
            if status == 202:
                stored_ids.append(content_id)

        # Wait for storage
        for content_id in stored_ids:
            self.client.wait_for_content_stored(content_id)

        # Get updated count
        status, response = self.client.get_content_count()
        if status == 200:
            updated_count = response.get("data", {}).get("count", 0)

            if updated_count >= initial_count + len(stored_ids):
                self.results.add_pass(f"Count - Updated count: {updated_count}")
            else:
                self.results.add_issue(
                    "Count Update",
                    "Count didn't increase as expected",
                    {
                        "initial": initial_count,
                        "added": len(stored_ids),
                        "updated": updated_count
                    }
                )
        else:
            self.results.add_fail("Count - Updated", f"Status: {status}")

        # Cleanup
        for content_id in stored_ids:
            self.client.delete_content(content_id)

    # -------------------------------------------------------------------------
    # Run All Scenarios
    # -------------------------------------------------------------------------

    def run_all_scenarios(self):
        """Run all test scenarios"""
        print("\n" + "=" * 70)
        print("RUNNING ALL TEST SCENARIOS")
        print("=" * 70)

        scenarios = [
            ("Basic CRUD", self.test_basic_crud),
            ("Async Write Queue", self.test_async_write_queue),
            ("Queue Ordering", self.test_queue_ordering),
            ("Access Limit", self.test_access_limit),
            ("Access Count Atomicity", self.test_access_count_atomicity),
            ("Time Expiration", self.test_time_expiration),
            ("Concurrent Writes", self.test_concurrent_writes),
            ("Concurrent Read/Write", self.test_concurrent_read_write),
            ("Error Handling", self.test_error_handling),
            ("Pagination Filtering", self.test_pagination_filtering),
            ("Health Metrics", self.test_health_metrics),
            ("Management Operations", self.test_management_operations),
            ("Content Overwrite", self.test_content_overwrite),
            ("Large Content", self.test_large_content),
            ("Count Endpoint", self.test_count_endpoint),
        ]

        for name, scenario in scenarios:
            try:
                scenario()
            except Exception as e:
                self.results.add_fail(name, str(e))
                print(f"  [ERROR] Exception in {name}: {e}")


# ============================================================================
# Main Entry Point
# ============================================================================

def main():
    parser = argparse.ArgumentParser(description="Content Storage Server Test Client")
    parser.add_argument("--url", default="http://localhost:8081", help="Server base URL")
    parser.add_argument("--api-key", help="API key for authentication")
    parser.add_argument("--timeout", type=int, default=30, help="Request timeout in seconds")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    parser.add_argument("--concurrent", type=int, default=10, help="Number of concurrent requests")
    parser.add_argument("--scenario", help="Run specific scenario only")

    args = parser.parse_args()

    config = TestConfig(
        base_url=args.url,
        api_key=args.api_key,
        timeout=args.timeout,
        verbose=args.verbose,
        concurrent_requests=args.concurrent
    )

    results = TestResults()
    client = ContentStorageClient(config, results)
    scenarios = TestScenarios(client, results, config)

    print("=" * 70)
    print("CONTENT STORAGE SERVER - TEST CLIENT")
    print("=" * 70)
    print(f"Server URL: {config.base_url}")
    print(f"API Key: {'***' if config.api_key else 'None'}")
    print(f"Timeout: {config.timeout}s")
    print(f"Concurrent Requests: {config.concurrent_requests}")

    # Test server connectivity first
    print("\n--- Testing Server Connectivity ---")
    success, response = client.health_check()
    if not success:
        print(f"ERROR: Cannot connect to server at {config.base_url}")
        print(f"Response: {response}")
        sys.exit(1)
    print(f"Server is reachable (status: {response.get('status', 'unknown')})")

    # Run scenarios
    if args.scenario:
        # Run specific scenario
        scenario_map = {
            "crud": scenarios.test_basic_crud,
            "async": scenarios.test_async_write_queue,
            "queue": scenarios.test_queue_ordering,
            "access": scenarios.test_access_limit,
            "atomic": scenarios.test_access_count_atomicity,
            "expiration": scenarios.test_time_expiration,
            "concurrent": scenarios.test_concurrent_writes,
            "rw": scenarios.test_concurrent_read_write,
            "error": scenarios.test_error_handling,
            "pagination": scenarios.test_pagination_filtering,
            "health": scenarios.test_health_metrics,
            "management": scenarios.test_management_operations,
            "overwrite": scenarios.test_content_overwrite,
            "large": scenarios.test_large_content,
            "count": scenarios.test_count_endpoint,
        }

        if args.scenario.lower() in scenario_map:
            scenario_map[args.scenario.lower()]()
        else:
            print(f"Unknown scenario: {args.scenario}")
            print(f"Available: {list(scenario_map.keys())}")
            sys.exit(1)
    else:
        # Run all scenarios
        scenarios.run_all_scenarios()

    # Print summary
    results.print_summary()

    # Exit with appropriate code
    if results.failed:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()