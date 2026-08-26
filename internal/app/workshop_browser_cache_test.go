package app

import (
	"fmt"
	"testing"
)

func withEmptyWorkshopCache(t *testing.T) {
	t.Helper()
	workshopCacheMu.Lock()
	oldCache := workshopCache
	oldSerial := workshopCacheAccessSerial
	workshopCache = make(map[string]WorkshopCacheItem)
	workshopCacheAccessSerial = 0
	workshopCacheMu.Unlock()
	t.Cleanup(func() {
		workshopCacheMu.Lock()
		workshopCache = oldCache
		workshopCacheAccessSerial = oldSerial
		workshopCacheMu.Unlock()
	})
}

func TestWorkshopCacheEvictsLeastRecentlyUsedEntry(t *testing.T) {
	withEmptyWorkshopCache(t)
	for i := 0; i < workshopCacheMaxEntries; i++ {
		setWorkshopCache(fmt.Sprintf("entry-%d", i), i)
	}
	if _, ok := getWorkshopCache("entry-0"); !ok {
		t.Fatal("expected initial entry in cache")
	}

	setWorkshopCache("new-entry", "new")
	if _, ok := getWorkshopCache("entry-1"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if value, ok := getWorkshopCache("entry-0"); !ok || value != 0 {
		t.Fatalf("recently used entry should remain: value=%v ok=%v", value, ok)
	}
	if value, ok := getWorkshopCache("new-entry"); !ok || value != "new" {
		t.Fatalf("new cache entry missing: value=%v ok=%v", value, ok)
	}
}
