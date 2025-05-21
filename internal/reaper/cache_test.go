package reaper

import (
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestInstanceMetricCache_PutAndGet(t *testing.T) {
	cache := NewInstanceMetricCache()

	// Define test data
	key := "test_key"
	data := metrics.Measurements{metrics.Measurement{"test_key": 42}}

	// Test Put
	cache.Put(key, data)
	// GetEpoch is implicitly set by Put if not already set.
	// For direct comparison, we might want to capture the data as it is in the cache or re-fetch it.
	// However, the original test just checks if it's not nil and epoch is close.

	// Test Get with valid key and age
	retrievedData := cache.Get(key, time.Second)
	assert.NotNil(t, retrievedData, "Expected data to be retrieved")
	// Compare content instead of epoch directly after Put, as Put might modify epoch.
	assert.Equal(t, data[0]["test_key"], retrievedData[0]["test_key"], "Data content should match")


	// Test Get with invalid key
	invalidKey := "invalid_key"
	retrievedData = cache.Get(invalidKey, time.Second)
	assert.Nil(t, retrievedData, "Expected nil for invalid key")

	// Test Get with expired age - this now also tests deletion
	// Need to ensure the item was in cache before this call.
	cache.Put(key, data) // Put it again
	time.Sleep(5 * time.Millisecond) // Ensure a small delay so that a negative or very small age definitely makes it stale.
	retrievedData = cache.Get(key, 1*time.Millisecond) 
	assert.Nil(t, retrievedData, "Expected nil for expired data, and it should be deleted")
	
	// Verify it was deleted
	retrievedDataAfterDelete := cache.Get(key, time.Second)
	assert.Nil(t, retrievedDataAfterDelete, "Expected nil as data should have been deleted due to staleness")


	// Test Get with empty key
	retrievedData = cache.Get("", time.Second)
	assert.Nil(t, retrievedData, "Expected nil for empty key")
}

func TestInstanceMetricCache_PutEmptyData(t *testing.T) {
	cache := NewInstanceMetricCache()

	// Test Put with empty data
	cache.Put("test_key_empty_data", metrics.Measurements{})
	retrievedData := cache.Get("test_key_empty_data", time.Second)
	assert.Nil(t, retrievedData, "Expected nil for empty data")

	data := metrics.Measurements{metrics.Measurement{}}
	// Test Put with empty key
	cache.Put("", data)
	retrievedData = cache.Get("", time.Second)
	assert.Nil(t, retrievedData, "Expected nil for empty key")
}

func TestInstanceMetricCache_Concurrency(t *testing.T) {
	cache := NewInstanceMetricCache()

	// Define test data
	key := "concurrent_key"
	data := metrics.Measurements{metrics.Measurement{"value": 123}}
	// Put sets the epoch, so no need to call Touch explicitly here for the test's purpose

	// Use goroutines to test concurrent access
	done := make(chan bool)
	go func() {
		cache.Put(key, data)
		done <- true
	}()
	go func() {
		_ = cache.Get(key, time.Second) // Attempt a read
		done <- true
	}()

	// Wait for goroutines to finish
	<-done
	<-done

	// Verify data is still accessible if Put happened and wasn't immediately evicted by a concurrent Get with short age
	retrievedData := cache.Get(key, time.Second)
	assert.NotNil(t, retrievedData, "Expected data to be retrieved after concurrent access")
	if retrievedData != nil {
		assert.Equal(t, data[0]["value"], retrievedData[0]["value"], "Data content should match after concurrent access")
	}
}

func TestInstanceMetricCache_Eviction(t *testing.T) {
	assert := assert.New(t) // Use assert helper

	cache := NewInstanceMetricCache()
	cacheKey := "eviction_test_key"
	testData := metrics.Measurements{metrics.Measurement{"value": "test_value"}}

	// Put data into cache
	cache.Put(cacheKey, testData)

	// 1. Verify immediate Get returns the data
	retrievedData := cache.Get(cacheKey, time.Second)
	assert.NotNil(retrievedData, "Data should be present immediately after Put")
	if retrievedData != nil {
		assert.Equal(testData[0]["value"], retrievedData[0]["value"], "Data content should match")
	}

	// 2. Wait for a period longer than the short maxAge to be used next
	time.Sleep(20 * time.Millisecond)

	// 3. Call Get with a maxAge that makes the item stale (e.g., 10ms < 20ms sleep)
	staleData := cache.Get(cacheKey, 10*time.Millisecond)
	assert.Nil(staleData, "Data should be stale and return nil, triggering eviction")

	// 4. To confirm deletion, try to Get again with a long maxAge
	// If it was deleted, it should not be found even with a long expiry
	dataAfterEviction := cache.Get(cacheKey, 10*time.Second)
	assert.Nil(dataAfterEviction, "Data should be nil after eviction, even with long maxAge")

	// Double check the internal map (not directly possible without exporting, but this Get confirms)
	// To be absolutely sure, we can try to "Put" another item and see if the old one is still there
	// This is not strictly necessary given the previous assert, but for completeness:
	cache.Lock() // Manual lock for direct inspection (if cache field were exported)
	_, exists := cache.cache[cacheKey]
	cache.Unlock()
	assert.False(exists, "Cache entry should not exist in the underlying map after eviction")
}
