package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dependencyRecord(key string, name string, dependencies ...string) ModDependencyRecord {
	refs := make([]ModDependencyRef, 0, len(dependencies))
	for _, dependency := range dependencies {
		refs = append(refs, ModDependencyRef{Key: dependency, Name: dependency})
	}
	return ModDependencyRecord{Key: key, Name: name, Dependencies: refs}
}

// 对齐 FireAxe `AddonCircularRefProblem`：环必须被找出、且只报一次。
func TestDependencyCyclesPure(t *testing.T) {
	cases := []struct {
		name    string
		records []ModDependencyRecord
		want    []string
	}{
		{
			name:    "没有依赖",
			records: nil,
			want:    nil,
		},
		{
			name: "链不是环",
			records: []ModDependencyRecord{
				dependencyRecord("a.vpk", "甲", "b.vpk"),
				dependencyRecord("b.vpk", "乙", "c.vpk"),
				dependencyRecord("c.vpk", "丙"),
			},
			want: nil,
		},
		{
			name: "两节点环",
			records: []ModDependencyRecord{
				dependencyRecord("a.vpk", "甲", "b.vpk"),
				dependencyRecord("b.vpk", "乙", "a.vpk"),
			},
			want: []string{"a.vpk->b.vpk"},
		},
		{
			name: "三节点环（旋转后只报一次）",
			records: []ModDependencyRecord{
				dependencyRecord("b.vpk", "乙", "c.vpk"),
				dependencyRecord("c.vpk", "丙", "a.vpk"),
				dependencyRecord("a.vpk", "甲", "b.vpk"),
			},
			want: []string{"a.vpk->b.vpk->c.vpk"},
		},
		{
			name: "自环",
			records: []ModDependencyRecord{
				dependencyRecord("solo.vpk", "独", "solo.vpk"),
			},
			want: []string{"solo.vpk"},
		},
		{
			name: "两个互不相干的环",
			records: []ModDependencyRecord{
				dependencyRecord("a.vpk", "甲", "b.vpk"),
				dependencyRecord("b.vpk", "乙", "a.vpk"),
				dependencyRecord("x.vpk", "X", "y.vpk"),
				dependencyRecord("y.vpk", "Y", "x.vpk"),
			},
			want: []string{"a.vpk->b.vpk", "x.vpk->y.vpk"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cycles := dependencyCycles(tc.records)
			got := make([]string, 0, len(cycles))
			for _, cycle := range cycles {
				got = append(got, strings.Join(cycle, "->"))
			}
			if len(got) != len(tc.want) {
				t.Fatalf("环数量 = %v, 期望 %v", got, tc.want)
			}
			for index := range tc.want {
				if got[index] != tc.want[index] {
					t.Fatalf("第 %d 个环 = %q, 期望 %q（全部：%v）", index, got[index], tc.want[index], got)
				}
			}
		})
	}
}

// 端到端：手写一份带环的 dependencies.json，体检要报出来，且不提供"一键修复"。
func TestDependencyCycleSurfacesInHealthCheck(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	rootDir := parityGameDir(addonsDir)

	store := modDependencyStore{Records: []ModDependencyRecord{
		dependencyRecord("a.vpk", "甲", "b.vpk"),
		dependencyRecord("b.vpk", "乙", "a.vpk"),
	}}
	raw, err := json.Marshal(store)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.configDir, "dependencies.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = rootDir

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("体检失败: %v", err)
	}
	var message string
	for _, issue := range report.Issues {
		if issue.Kind == modHealthKindDependencyCycle {
			message = issue.Message
		}
	}
	if message == "" {
		t.Fatalf("应报出依赖成环: %+v", report.Issues)
	}
	if !strings.Contains(message, "甲 → 乙 → 甲") {
		t.Fatalf("环的报告应写清链路，实际 %q", message)
	}
}
