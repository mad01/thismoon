package vault

import "testing"

func TestListStatusFilterUsesTheTierDefault(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"notes/bare.md":     "---\ntitle: Bare\n---\nNo status line.\n",
		"notes/disputed.md": "---\ntitle: Disputed\nstatus: disputed\n---\nDoubted.\n",
		"inbox/bare.md":     "---\ntitle: Bare capture\n---\nNo status line.\n",
	})
	tests := []struct {
		tier   Tier
		status string
		want   string
	}{
		{TierNotes, "validated", "notes/bare.md"},
		{TierNotes, "Validated", "notes/bare.md"},
		{TierNotes, "disputed", "notes/disputed.md"},
		{TierInbox, "raw", "inbox/bare.md"},
	}
	for _, tt := range tests {
		listing, err := v.List(ListOptions{Tier: tt.tier, Status: tt.status})
		if err != nil {
			t.Fatal(err)
		}
		if len(listing.Notes) != 1 || listing.Notes[0].Path != tt.want {
			t.Errorf("List(%s, status %s) = %+v, want %s alone", tt.tier, tt.status, listing.Notes, tt.want)
		}
	}
}
