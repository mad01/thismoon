package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// fingerprintTTL is how long one fingerprint of every vault answers
// /api/state and the model's freshness check before the vaults are walked
// again. The UI polls every 10 seconds, so a change shows within one poll.
const fingerprintTTL = 2 * time.Second

// layoutVersion is part of every layout key. Bump it when the graph's node
// ids, the layout file's shape or the layout's scale change, so no old
// layout is reused. Version 2 is the d3-force layout, which spaces nodes in
// pixels at zoom 1 where version 1's ForceAtlas2 used its own units.
// Version 3 holds the notes on one island per community, and the UI reads
// the islands back out of a saved layout, so a version 2 one is not used.
const layoutVersion = 3

// vaultPrint is one vault's fingerprint, the same mtime and size digest the
// literal index's freshness check computes.
type vaultPrint struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

// model is every readable vault read once: the snapshot behind the graph,
// the notes, the stale queue and link resolution. It is immutable once
// built, so handlers share it without a lock.
type model struct {
	key    string
	prints []vaultPrint
	// set is the vaults that could be read, in config order.
	set *vault.Set
	sn  *vault.Snapshot
}

// current returns the model of the vaults as they are now, rebuilding it
// when a fingerprint changed. Callers wait on one mutex while it rebuilds.
func (s *Server) current() (*model, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prints := s.fingerprintsLocked()
	key := layoutKey(prints, false)
	if s.model != nil && s.model.key == key {
		return s.model, nil
	}
	m, err := s.buildModel(prints, key)
	if err != nil {
		return nil, err
	}
	s.model = m
	return m, nil
}

// fingerprints returns every vault's fingerprint, memoized for
// fingerprintTTL.
func (s *Server) fingerprints() []vaultPrint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fingerprintsLocked()
}

func (s *Server) fingerprintsLocked() []vaultPrint {
	now := s.now()
	if s.prints != nil && now.Sub(s.printsAt) < fingerprintTTL && !now.Before(s.printsAt) {
		return s.prints
	}
	vaults := s.lib.Set().Vaults()
	prints := make([]vaultPrint, 0, len(vaults))
	for _, v := range vaults {
		fp, _, err := search.ComputeFingerprint(v.Root)
		if err != nil {
			fp = ""
		}
		if readErr := readable(v.Root); readErr != nil {
			fp = "unreadable"
		}
		prints = append(prints, vaultPrint{Name: v.Name, Fingerprint: fp})
	}
	s.prints, s.printsAt = prints, now
	return prints
}

// buildModel snapshots the vaults that can be read now. One that cannot is
// left out, and /api/doctor says why.
func (s *Server) buildModel(prints []vaultPrint, key string) (*model, error) {
	var vaults []*vault.Vault
	for _, v := range s.lib.Set().Vaults() {
		if readable(v.Root) == nil {
			vaults = append(vaults, v)
		}
	}
	set := vault.NewSet(vaults...)
	sn, err := set.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("read the vaults: %w", err)
	}
	return &model{key: key, prints: prints, set: set, sn: sn}, nil
}

// readable reports whether the directory at root can be listed.
func readable(root string) error {
	_, err := os.ReadDir(root)
	return err
}

// layoutKey is the sha256 of the layout version, each vault's
// name|fingerprint and the topics flag, in hex.
func layoutKey(prints []vaultPrint, topics bool) string {
	h := sha256.New()
	fmt.Fprintf(h, "brain-layout-v%d\n", layoutVersion)
	for _, p := range prints {
		fmt.Fprintf(h, "%s|%s\n", p.Name, p.Fingerprint)
	}
	fmt.Fprintf(h, "topics=%t\n", topics)
	return hex.EncodeToString(h.Sum(nil))
}

// scope returns the model's vaults, or the one name names. A vault that is
// not configured, or could not be read, is a 400.
func (m *model) scope(name string) (*vault.Set, error) {
	if name == "" {
		return m.set, nil
	}
	return m.set.Filter([]string{name})
}
