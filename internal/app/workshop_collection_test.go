package app

import (
	"fmt"
	"strings"
	"testing"
)

func workshopDetailWithChildren(id string, childIDs ...string) WorkshopFileDetails {
	detail := WorkshopFileDetails{Result: 1, PublishedFileId: id, Title: "item-" + id}
	for index, childID := range childIDs {
		detail.Children = append(detail.Children, WorkshopChild{
			PublishedFileId: childID,
			SortOrder:       index,
			FileType:        2,
		})
	}
	return detail
}

// parseWorkshopPayloadIDs 解析后端批量请求里的 ID（形如 [1,2,3]）。
func parseWorkshopPayloadIDs(payload string) []string {
	trimmed := strings.Trim(strings.TrimSpace(payload), "[]")
	if trimmed == "" {
		return nil
	}
	ids := make([]string, 0, 8)
	for _, part := range strings.Split(trimmed, ",") {
		id := strings.Trim(strings.TrimSpace(part), `"`)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func newFakeWorkshopFetcher(t *testing.T, catalog map[string]WorkshopFileDetails) (workshopDetailFetcher, *[][]string) {
	t.Helper()
	requests := make([][]string, 0)
	fetcher := func(payload string) ([]WorkshopFileDetails, error) {
		ids := parseWorkshopPayloadIDs(payload)
		requests = append(requests, ids)
		details := make([]WorkshopFileDetails, 0, len(ids))
		for _, id := range ids {
			detail, ok := catalog[id]
			if !ok {
				return nil, fmt.Errorf("unexpected id %s", id)
			}
			details = append(details, detail)
		}
		return details, nil
	}
	return fetcher, &requests
}

func workshopItemIDs(items []WorkshopFileDetails) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.PublishedFileId)
	}
	return ids
}

func TestCollectWorkshopGroupItemsExpandsNestedCollections(t *testing.T) {
	catalog := map[string]WorkshopFileDetails{
		"1": workshopDetailWithChildren("1", "2", "3"),
		"2": workshopDetailWithChildren("2", "4", "5"),
		"3": workshopDetailWithChildren("3"),
		"4": workshopDetailWithChildren("4"),
		"5": workshopDetailWithChildren("5", "6"),
		"6": workshopDetailWithChildren("6"),
	}
	fetch, requests := newFakeWorkshopFetcher(t, catalog)

	items, truncated, err := collectWorkshopGroupItems("1", catalog["1"], fetch, 20)
	if err != nil {
		t.Fatalf("collect items: %v", err)
	}
	if truncated {
		t.Fatal("did not expect truncation for a small collection tree")
	}

	want := []string{"1", "2", "3", "4", "5", "6"}
	got := workshopItemIDs(items)
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for index, id := range want {
		if got[index] != id {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}

	// 子合集只应展开一次：第二轮请求拿到 4、5，第三轮拿到 6。
	if len(*requests) != 3 {
		t.Fatalf("expected three batched requests, got %#v", *requests)
	}
	if ids := (*requests)[1]; len(ids) != 2 || ids[0] != "4" || ids[1] != "5" {
		t.Fatalf("expected the second request to expand the sub-collection, got %#v", ids)
	}
}

func TestCollectWorkshopGroupItemsStopsOnCircularCollections(t *testing.T) {
	catalog := map[string]WorkshopFileDetails{
		"1": workshopDetailWithChildren("1", "2"),
		"2": workshopDetailWithChildren("2", "1"),
	}
	fetch, _ := newFakeWorkshopFetcher(t, catalog)

	items, truncated, err := collectWorkshopGroupItems("1", catalog["1"], fetch, 20)
	if err != nil {
		t.Fatalf("collect items: %v", err)
	}
	if truncated {
		t.Fatal("a circular reference should terminate without truncation")
	}
	if ids := workshopItemIDs(items); len(ids) != 2 || ids[0] != "1" || ids[1] != "2" {
		t.Fatalf("expected the cycle to be cut after two items, got %#v", ids)
	}
}

func TestCollectWorkshopGroupItemsTruncatesDeepNesting(t *testing.T) {
	catalog := map[string]WorkshopFileDetails{
		"1": workshopDetailWithChildren("1", "2"),
		"2": workshopDetailWithChildren("2", "3"),
		"3": workshopDetailWithChildren("3", "4"),
		"4": workshopDetailWithChildren("4", "5"),
		"5": workshopDetailWithChildren("5"),
	}
	fetch, _ := newFakeWorkshopFetcher(t, catalog)

	items, truncated, err := collectWorkshopGroupItems("1", catalog["1"], fetch, 2)
	if err != nil {
		t.Fatalf("collect items: %v", err)
	}
	if !truncated {
		t.Fatal("expected deep nesting to be reported as truncated")
	}
	if ids := workshopItemIDs(items); len(ids) != 4 {
		t.Fatalf("expected the traversal to stop after the allowed expansions, got %#v", ids)
	}
}
