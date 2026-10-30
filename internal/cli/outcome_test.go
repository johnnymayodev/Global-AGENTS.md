package cli

import (
	"testing"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/ui"
)

func TestWizardOutcome(t *testing.T) {
	notice := []string{"Created blank instructions file: /tmp/AGENTS.md"}
	cases := []struct {
		name   string
		result ui.WizardResult
		want   string
	}{
		{name: "cancelled", result: ui.WizardResult{Cancelled: true}, want: "Cancelled. No changes made."},
		{
			name:   "cancelled after setup",
			result: ui.WizardResult{Cancelled: true, Created: true},
			want:   "Cancelled. No links were changed.",
		},
		{
			name:   "cancelled with notices but no creation",
			result: ui.WizardResult{Cancelled: true, Notices: notice},
			want:   "Cancelled. No changes made.",
		},
		{name: "no changes", result: ui.WizardResult{NoChanges: true}, want: "No changes made."},
		{
			name:   "no changes after setup",
			result: ui.WizardResult{NoChanges: true, Created: true},
			want:   "No links were changed.",
		},
		{
			name:   "no changes with notices but no creation",
			result: ui.WizardResult{NoChanges: true, Notices: notice},
			want:   "No changes made.",
		},
		{name: "applied", result: ui.WizardResult{Applied: true}, want: ""},
		{name: "unfinished", result: ui.WizardResult{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wizardOutcome(tc.result); got != tc.want {
				t.Fatalf("wizardOutcome(%+v) = %q, want %q", tc.result, got, tc.want)
			}
		})
	}
}

func TestLinkNoChangesLine(t *testing.T) {
	cases := []struct {
		name        string
		created     bool
		interactive bool
		want        string
	}{
		{name: "plain", created: false, interactive: false, want: "No changes made."},
		{name: "plain after setup", created: true, interactive: false, want: "No changes made."},
		{name: "interactive", created: false, interactive: true, want: "No changes made."},
		{name: "interactive after setup", created: true, interactive: true, want: "No links were changed."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkNoChangesLine(tc.created, tc.interactive); got != tc.want {
				t.Fatalf("linkNoChangesLine(%v, %v) = %q, want %q", tc.created, tc.interactive, got, tc.want)
			}
		})
	}
}
