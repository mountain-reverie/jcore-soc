package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"time"
)

// watchInterval is the mtime poll period for -watch.
const watchInterval = 500 * time.Millisecond

// watchedFiles returns the board input files that -watch monitors: the board's
// design.yaml plus the shared include yamls (targets/boards/*.yaml), deduped and
// sorted. Editing any of them triggers a regenerate.
func watchedFiles(root, name string) []string {
	set := map[string]bool{
		filepath.Join(root, "targets/boards", name, "design.yaml"): true,
	}
	// The glob is targets/boards/*.yaml — the shared includes only (top-level,
	// not recursive); each board's own design.yaml lives in a subdirectory and is
	// added explicitly above.
	shared, _ := filepath.Glob(filepath.Join(root, "targets/boards", "*.yaml"))
	for _, p := range shared {
		set[p] = true
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// snapshot maps each path to its modtime (a missing file -> the zero time).
func snapshot(paths []string) map[string]time.Time {
	m := make(map[string]time.Time, len(paths))
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			m[p] = fi.ModTime()
		} else {
			m[p] = time.Time{}
		}
	}
	return m
}

// changed reports whether any path's modtime differs between two snapshots.
func changed(prev, cur map[string]time.Time) bool {
	if len(prev) != len(cur) {
		return true
	}
	for p, t := range cur {
		if !prev[p].Equal(t) {
			return true
		}
	}
	return false
}

// runWatch generates the board once, then regenerates whenever a watched input
// file changes, until interrupted (SIGINT). Generate errors are printed and
// watching continues (a dev convenience).
func runWatch(root, name, outDir string) error {
	if err := generateBoard(root, name, outDir); err != nil {
		return err
	}
	paths := watchedFiles(root, name)
	prev := snapshot(paths)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	fmt.Fprintf(os.Stderr, "socgen: watching %s (Ctrl-C to stop)\n", name)
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-sig:
			fmt.Fprintln(os.Stderr, "socgen: stopped")
			return nil
		case <-ticker.C:
			cur := snapshot(paths)
			if changed(prev, cur) {
				prev = cur
				if err := generateBoard(root, name, outDir); err != nil {
					fmt.Fprintln(os.Stderr, "socgen: regenerate error:", err)
				} else {
					fmt.Fprintf(os.Stderr, "socgen: regenerated %s\n", name)
				}
			}
		}
	}
}
