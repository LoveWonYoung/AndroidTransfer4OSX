package main

import "testing"

func TestBuildTransferQueueSnapshot(t *testing.T) {
	queue := []queueItem{
		{LocalPath: "/tmp/a", Status: "成功 1 MB/s"},
		{LocalPath: "/tmp/b", Status: "失败"},
		{LocalPath: "/tmp/c", Status: "超时"},
		{LocalPath: "/tmp/d", Status: "已取消"},
		{LocalPath: "/tmp/e", Status: "待重试"},
		{LocalPath: "/tmp/e", Status: "待传输"},
	}

	all := buildTransferQueueSnapshot(queue, false)
	if len(all) != len(queue) {
		t.Fatalf("full snapshot length = %d, want %d", len(all), len(queue))
	}

	retry := buildTransferQueueSnapshot(queue, true)
	wantIndexes := []int{1, 2, 3, 4}
	if len(retry) != len(wantIndexes) {
		t.Fatalf("retry snapshot length = %d, want %d", len(retry), len(wantIndexes))
	}
	for i, want := range wantIndexes {
		if retry[i].QueueIndex != want {
			t.Fatalf("retry[%d].QueueIndex = %d, want %d", i, retry[i].QueueIndex, want)
		}
	}
}

func TestCountRetryableQueueItems(t *testing.T) {
	queue := []queueItem{
		{Status: "失败"},
		{Status: "超时"},
		{Status: "已取消"},
		{Status: "待重试"},
		{Status: "成功 1 MB/s"},
		{Status: "待传输"},
	}

	if got := countRetryableQueueItems(queue); got != 4 {
		t.Fatalf("countRetryableQueueItems() = %d, want 4", got)
	}
}
