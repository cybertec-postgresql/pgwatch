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

	// Test Get with valid key and age
	retrievedData := cache.Get(key, time.Second)
	assert.NotNil(t, retrievedData, "Expected data to be retrieved")
	assert.Equal(t, data.GetEpoch(), retrievedData.GetEpoch(), "Epoch times should match")

	// Test Get with invalid key
	invalidKey := "invalid_key"
	retrievedData = cache.Get(invalidKey, time.Second)
	assert.Nil(t, retrievedData, "Expected nil for invalid key")

	// Test Get with expired age
	retrievedData = cache.Get(key, -time.Second) // Negative age to simulate expiration
	assert.Nil(t, retrievedData, "Expected nil for expired data")

	// Test Get with empty key
	retrievedData = cache.Get("", time.Second)
	assert.Nil(t, retrievedData, "Expected nil for empty key")
}

func TestInstanceMetricCache_PutEmptyData(t *testing.T) {
	cache := NewInstanceMetricCache()

	// Test Put with empty data
	cache.Put("test_key", metrics.Measurements{})
	retrievedData := cache.Get("test_key", time.Second)
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
	key := "test_key"
	data := metrics.Measurements{metrics.Measurement{}}
	data.Touch()

	// Use goroutines to test concurrent access
	done := make(chan bool)
	go func() {
		cache.Put(key, data)
		done <- true
	}()
	go func() {
		_ = cache.Get(key, time.Second)
		done <- true
	}()

	// Wait for goroutines to finish
	<-done
	<-done

	// Verify data is still accessible
	retrievedData := cache.Get(key, time.Second)
	assert.NotNil(t, retrievedData, "Expected data to be retrieved after concurrent access")
}
