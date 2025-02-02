package kv

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestKv(t *testing.T) *Kv {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to in-memory db: %v", err)
	}
	// Create a new Kv instance with the test table name.
	kvInstance, err := NewKv(&db, WithDBName("test_kv"))
	if err != nil {
		t.Fatalf("failed to create kv instance: %v", err)
	}
	return kvInstance
}

func TestSetAndGet(t *testing.T) {
	kvInstance := setupTestKv(t)
	ctx := context.Background()

	key, value := "testkey", "testvalue"
	ttl := 5 * time.Second

	err := kvInstance.SetWithTTL(ctx, key, value, ttl)
	assert.NoError(t, err, "SetWithTTL should not error")

	item, err := kvInstance.Get(ctx, key)
	assert.NoError(t, err, "Get should not error")
	assert.Equal(t, key, item.Key)
	assert.Equal(t, value, item.Value)
}

func TestKeyExpiration(t *testing.T) {
	kvInstance := setupTestKv(t)
	ctx := context.Background()

	key, value := "expirekey", "expirevalue"
	ttl := 1 * time.Second

	err := kvInstance.SetWithTTL(ctx, key, value, ttl)
	assert.NoError(t, err, "SetWithTTL should not error")

	// Wait for the key to expire.
	time.Sleep(2 * time.Second)
	_, err = kvInstance.Get(ctx, key)
	assert.Error(t, err, "key should be expired")
}

func TestExistsAndDel(t *testing.T) {
	kvInstance := setupTestKv(t)
	ctx := context.Background()

	key, value := "existkey", "existvalue"
	err := kvInstance.SetWithExpireAt(ctx, key, value, time.Now().Add(10*time.Second))
	assert.NoError(t, err, "SetWithExpireAt should not error")

	exists, err := kvInstance.Exists(ctx, key)
	assert.NoError(t, err, "Exists should not error")
	assert.True(t, exists, "key should exist")

	err = kvInstance.Del(ctx, key)
	assert.NoError(t, err, "Del should not error")

	exists, err = kvInstance.Exists(ctx, key)
	assert.NoError(t, err, "Exists after deletion should not error")
	assert.False(t, exists, "key should not exist after deletion")
}
