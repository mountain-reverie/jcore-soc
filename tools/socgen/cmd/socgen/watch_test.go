package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChanged(t *testing.T) {
	base := map[string]time.Time{"a": time.Unix(1, 0), "b": time.Unix(2, 0)}
	same := map[string]time.Time{"a": time.Unix(1, 0), "b": time.Unix(2, 0)}
	if changed(base, same) {
		t.Errorf("identical snapshots should not be changed")
	}
	diff := map[string]time.Time{"a": time.Unix(9, 0), "b": time.Unix(2, 0)}
	if !changed(base, diff) {
		t.Errorf("differing modtime should be changed")
	}
	fewer := map[string]time.Time{"a": time.Unix(1, 0)}
	if !changed(base, fewer) {
		t.Errorf("differing count should be changed")
	}
}

func TestWatchedFilesTurtle(t *testing.T) {
	root := os.Getenv("JCORE_SOC_ROOT")
	if root == "" {
		t.Skip("JCORE_SOC_ROOT not set")
	}
	got := watchedFiles(root, "turtle_1v0")
	wantBoard := filepath.Join(root, "targets/boards/turtle_1v0/design.yaml")
	wantShared := filepath.Join(root, "targets/boards/common_device_classes.yaml")
	haveBoard, haveShared, sorted := false, false, true
	for i, p := range got {
		if p == wantBoard {
			haveBoard = true
		}
		if p == wantShared {
			haveShared = true
		}
		if i > 0 && got[i-1] > p {
			sorted = false
		}
	}
	if !haveBoard {
		t.Errorf("watchedFiles missing the board design.yaml; got %v", got)
	}
	if !haveShared {
		t.Errorf("watchedFiles missing shared include; got %v", got)
	}
	if !sorted {
		t.Errorf("watchedFiles not sorted: %v", got)
	}
}
