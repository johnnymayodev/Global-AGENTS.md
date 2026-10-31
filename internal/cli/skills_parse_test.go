package cli

import "testing"

func TestParseSkillsInvocation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want skillsInvocation
		ok   bool
	}{
		{name: "manage", args: nil, want: skillsInvocation{mode: skillsModeManage}, ok: true},
		{name: "manage all", args: []string{"--all"}, want: skillsInvocation{mode: skillsModeManage, showAll: true}, ok: true},
		{name: "list", args: []string{"--list"}, want: skillsInvocation{mode: skillsModeList}, ok: true},
		{name: "list all", args: []string{"--list", "--all"}, want: skillsInvocation{mode: skillsModeList, showAll: true}, ok: true},
		{name: "link", args: []string{"link", "git-release"}, want: skillsInvocation{mode: skillsModeLink, name: "git-release"}, ok: true},
		{name: "unlink", args: []string{"unlink", "git-release"}, want: skillsInvocation{mode: skillsModeUnlink, name: "git-release"}, ok: true},
		{name: "link terminator", args: []string{"link", "--", "-leading-dash"}, want: skillsInvocation{mode: skillsModeLink, name: "-leading-dash"}, ok: true},
		{name: "unlink terminator flag-like name", args: []string{"unlink", "--", "--all"}, want: skillsInvocation{mode: skillsModeUnlink, name: "--all"}, ok: true},
		{name: "help short", args: []string{"-h"}, want: skillsInvocation{mode: skillsModeHelp}, ok: true},
		{name: "help long", args: []string{"--help"}, want: skillsInvocation{mode: skillsModeHelp}, ok: true},

		{name: "unknown command", args: []string{"bogus"}},
		{name: "list without dashes", args: []string{"list"}},
		{name: "list unknown flag", args: []string{"--list", "--help"}},
		{name: "list extra argument", args: []string{"--list", "--all", "extra"}},
		{name: "manage all help", args: []string{"--all", "--help"}},
		{name: "manage all extra argument", args: []string{"--all", "extra"}},
		{name: "help extra argument", args: []string{"--help", "extra"}},
		{name: "link help", args: []string{"link", "--help"}},
		{name: "unlink short help", args: []string{"unlink", "-h"}},
		{name: "link flag", args: []string{"link", "--all"}},
		{name: "link unknown flag", args: []string{"link", "-x"}},
		{name: "link missing name", args: []string{"link"}},
		{name: "link extra name", args: []string{"link", "a", "b"}},
		{name: "link terminator only", args: []string{"link", "--"}},
		{name: "link terminator extra name", args: []string{"link", "--", "a", "b"}},
		{name: "unlink flag", args: []string{"unlink", "--all"}},
		{name: "lone terminator", args: []string{"--"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSkillsInvocation(tt.args)
			if ok != tt.ok {
				t.Fatalf("parseSkillsInvocation(%q) ok = %t, want %t", tt.args, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("parseSkillsInvocation(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}
