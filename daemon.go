package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

type tab struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Number      int    `json:"number"`
	Label       string `json:"label"`
	Focused     bool   `json:"focused"`
}

type pane struct {
	PaneID        string `json:"pane_id"`
	TabID         string `json:"tab_id"`
	Focused       bool   `json:"focused"`
	CWD           string `json:"cwd"`
	ForegroundCWD string `json:"foreground_cwd"`
}

type tabListResult struct {
	Tabs []tab `json:"tabs"`
}

type paneListResult struct {
	Panes []pane `json:"panes"`
}

// daemon holds the single piece of session state: lastSeen maps a tab_id to the
// label the daemon last observed for it. The locked set records tabs the user
// manually renamed mid-session; those are left alone for the rest of the run.
type daemon struct {
	socket   string
	log      *log.Logger
	lastSeen map[string]string
	locked   map[string]bool
}

func newDaemon(socket string, logger *log.Logger) *daemon {
	return &daemon{
		socket:   socket,
		log:      logger,
		lastSeen: make(map[string]string),
		locked:   make(map[string]bool),
	}
}

// seed performs an initial tab.list pass BEFORE the ticker starts and records
// each existing tab's current label as the baseline. It makes no lock decision.
// This prevents the mass-lock bug: a tab is never locked just for existing;
// locking only happens for a label change observed DURING the running session.
func (d *daemon) seed() error {
	tabs, err := d.tabList()
	if err != nil {
		return err
	}
	for _, t := range tabs {
		d.lastSeen[t.TabID] = t.Label
	}
	d.log.Printf("seeded %d tab(s)", len(tabs))
	return nil
}

// restoreLocks re-loads persisted locked tabs from disk and re-locks those that
// still exist as live tabs. Stale ids from a previous Herdr session (different
// tab_ids) are dropped. Called after seed() so existing tabs are already
// baselined; this only marks the persisted ones as locked.
func (d *daemon) restoreLocks() {
	tabs, err := d.tabList()
	if err != nil {
		return
	}
	live := make(map[string]bool, len(tabs))
	for _, t := range tabs {
		live[t.TabID] = true
	}
	for id := range loadLocks() {
		if live[id] {
			d.locked[id] = true
		}
	}
	if n := len(d.locked); n > 0 {
		d.log.Printf("restored %d persisted locked tab(s)", n)
	}
}

// loadLocks reads the persisted locked-tab ids. A missing or unparseable file
// yields an empty map (never an error).
func loadLocks() map[string]bool {
	out := map[string]bool{}
	b, err := os.ReadFile(locksPath())
	if err != nil {
		return out
	}
	var s struct {
		LockedTabs []string `json:"locked_tabs"`
	}
	if json.Unmarshal(b, &s) != nil {
		return out
	}
	for _, id := range s.LockedTabs {
		out[id] = true
	}
	return out
}

// saveLocks atomically writes the current locked set to disk.
func (d *daemon) saveLocks() {
	ids := make([]string, 0, len(d.locked))
	for id := range d.locked {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	b, err := json.MarshalIndent(struct {
		LockedTabs []string `json:"locked_tabs"`
	}{LockedTabs: ids}, "", "  ")
	if err != nil {
		return
	}
	tmp := locksPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		d.log.Printf("saveLocks write tmp: %v", err)
		return
	}
	if err := os.Rename(tmp, locksPath()); err != nil {
		d.log.Printf("saveLocks rename: %v", err)
	}
}

func (d *daemon) poll() error {
	tabs, err := d.tabList()
	if err != nil {
		return fmt.Errorf("tab.list: %w", err)
	}
	panes, err := d.paneList()
	if err != nil {
		return fmt.Errorf("pane.list: %w", err)
	}
	byTab := make(map[string][]pane, len(tabs))
	for _, p := range panes {
		byTab[p.TabID] = append(byTab[p.TabID], p)
	}

	for _, t := range tabs {
		if d.locked[t.TabID] {
			continue
		}
		current := t.Label

		desired, ok := desiredLabel(byTab[t.TabID])
		if !ok {
			continue // no cwd to derive a name from; leave the tab as-is
		}

		// LOCK DETECTION (observed change): if we have a baseline label for this
		// tab, the current label differs from that baseline, AND it also differs
		// from what the daemon would set, then the user changed it mid-session.
		if prev, seen := d.lastSeen[t.TabID]; seen && current != prev && current != desired {
			d.locked[t.TabID] = true
			d.log.Printf("locked %s (user renamed to %q)", t.TabID, current)
			d.saveLocks()
			continue
		}

		// RENAME if needed, then keep lastSeen in sync with the latest label.
		if desired != current {
			if err := d.rename(t.TabID, desired); err != nil {
				d.log.Printf("rename %s failed: %v", t.TabID, err)
				d.lastSeen[t.TabID] = current
				continue
			}
			d.lastSeen[t.TabID] = desired
			d.log.Printf("renamed %s -> %q", t.TabID, desired)
		} else {
			d.lastSeen[t.TabID] = current
		}
	}
	return nil
}

// desiredLabel resolves the basename of the focused pane's cwd for a tab.
// It prefers foreground_cwd (the foreground process's cwd) and falls back to
// cwd. The focused pane is the one with focused=true; if none is focused, the
// first pane is used. Returns ok=false if there is no pane or no cwd.
func desiredLabel(panes []pane) (string, bool) {
	if len(panes) == 0 {
		return "", false
	}
	picked := &panes[0]
	for i := range panes {
		if panes[i].Focused {
			picked = &panes[i]
			break
		}
	}
	cwd := picked.ForegroundCWD
	if cwd == "" {
		cwd = picked.CWD
	}
	if cwd == "" {
		return "", false
	}
	return filepath.Base(cwd), true
}

func (d *daemon) tabList() ([]tab, error) {
	res, err := rpcCall(d.socket, "tab.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out tabListResult
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	return out.Tabs, nil
}

func (d *daemon) paneList() ([]pane, error) {
	res, err := rpcCall(d.socket, "pane.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out paneListResult
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	return out.Panes, nil
}

func (d *daemon) rename(tabID, label string) error {
	_, err := rpcCall(d.socket, "tab.rename", map[string]any{"tab_id": tabID, "label": label})
	return err
}
