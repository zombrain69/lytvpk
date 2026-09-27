package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// 对齐上游 361dc9a：工坊详情要能把"依赖物品"（required_items）带出来给界面用。
func TestWorkshopItemDetailDecodesRequiredItems(t *testing.T) {
	const payload = `{
	  "response": {
	    "publishedfiledetails": [{
	      "publishedfileid": "999",
	      "title": "带依赖的武器",
	      "file_url": "https://example.invalid/999.vpk",
	      "preview_url": "https://example.invalid/999.jpg",
	      "child_items": [],
	      "required_items": [
	        {"publishedfileid": "111", "title": "前置库", "preview_url": "https://example.invalid/111.jpg", "views": 12},
	        {"publishedfileid": "222", "title": "另一个前置", "preview_url": "", "views": 0}
	      ]
	    }]
	  }
	}`

	var decoded SteamDetailResponse
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("解析详情响应失败: %v", err)
	}
	if len(decoded.Response.PublishedFileDetails) != 1 {
		t.Fatalf("详情数量 = %d", len(decoded.Response.PublishedFileDetails))
	}
	item := decoded.Response.PublishedFileDetails[0]
	if len(item.RequiredItems) != 2 {
		t.Fatalf("required_items 未解码: %+v", item.RequiredItems)
	}
	if item.RequiredItems[0].PublishedFileId != "111" || item.RequiredItems[0].Title != "前置库" {
		t.Fatalf("依赖项内容错误: %+v", item.RequiredItems[0])
	}
	if item.RequiredItems[0].Views != 12 {
		t.Fatalf("依赖项统计丢失: %+v", item.RequiredItems[0])
	}
}

// 优选 IP 关闭时：图片处理不应丢字段，也不应把缓存里的原始切片改掉。
func TestProcessWorkshopDetailImagesKeepsRequiredItemsAndCopiesSlices(t *testing.T) {
	app := &App{}
	original := WorkshopItemDetail{
		PublishedFileId: "999",
		Title:           "带依赖的武器",
		PreviewUrl:      "https://example.invalid/999.jpg",
		Previews:        []WorkshopPreviewImage{{PreviewUrl: "https://example.invalid/p1.jpg"}},
		ChildItems:      []WorkshopPreviewItem{{PublishedFileId: "1", PreviewUrl: "https://example.invalid/c1.jpg"}},
		RequiredItems:   []WorkshopPreviewItem{{PublishedFileId: "111", Title: "前置库", PreviewUrl: "https://example.invalid/111.jpg"}},
	}

	processed := app.processWorkshopDetailImages(original)
	if len(processed.RequiredItems) != 1 || processed.RequiredItems[0].PublishedFileId != "111" {
		t.Fatalf("依赖项丢失: %+v", processed.RequiredItems)
	}
	if processed.RequiredItems[0].PreviewUrl != "https://example.invalid/111.jpg" {
		t.Fatalf("未开启优选 IP 时不应改写图片地址: %q", processed.RequiredItems[0].PreviewUrl)
	}
	if len(processed.ChildItems) != 1 || len(processed.Previews) != 1 {
		t.Fatalf("子项/预览图丢失: %+v %+v", processed.ChildItems, processed.Previews)
	}

	// 改副本不应影响原始数据（缓存里存的是原始 URL）。
	processed.RequiredItems[0].PreviewUrl = "changed"
	processed.ChildItems[0].PreviewUrl = "changed"
	if strings.Contains(original.RequiredItems[0].PreviewUrl, "changed") {
		t.Fatal("依赖项切片被别名修改")
	}
	if strings.Contains(original.ChildItems[0].PreviewUrl, "changed") {
		t.Fatal("子项切片被别名修改")
	}
}
