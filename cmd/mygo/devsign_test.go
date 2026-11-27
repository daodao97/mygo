package main

import "testing"

func TestDevelopmentIdentity(t *testing.T) {
	out := `  1) 0000000000000000000000000000000000000001 "Developer ID Application: Jane Doe (AAAA)"
  2) 0000000000000000000000000000000000000002 "Mac Developer: Jane Doe (OLD)"
  3) 0000000000000000000000000000000000000003 "Apple Development: jane@example.com (BBBB)"
  4) 0000000000000000000000000000000000000004 "Apple Development: Jane Doe (AAAA)"
     4 valid identities found
`
	for _, tt := range []struct {
		out, team, hash, name string
	}{
		{out, "", "0000000000000000000000000000000000000003", "Apple Development: jane@example.com (BBBB)"},
		{out, "AAAA", "0000000000000000000000000000000000000004", "Apple Development: Jane Doe (AAAA)"},
		{out, "OLD", "0000000000000000000000000000000000000002", "Mac Developer: Jane Doe (OLD)"},
		{out, "CCCC", "0000000000000000000000000000000000000003", "Apple Development: jane@example.com (BBBB)"},
		{`  1) 0000000000000000000000000000000000000002 "Mac Developer: Jane Doe (OLD)"`, "", "0000000000000000000000000000000000000002", "Mac Developer: Jane Doe (OLD)"},
		{`  1) 0000000000000000000000000000000000000001 "Developer ID Application: Jane Doe (AAAA)"`, "AAAA", "", ""},
		{"     0 valid identities found\n", "", "", ""},
	} {
		if hash, name := developmentIdentity(tt.out, nil, tt.team); hash != tt.hash || name != tt.name {
			t.Errorf("developmentIdentity(%q, %q) = %q, %q; want %q, %q", tt.out, tt.team, hash, name, tt.hash, tt.name)
		}
	}
}

func TestIdentityKind(t *testing.T) {
	for in, want := range map[string]string{
		"Apple Development: Jane Doe (AAAA)":         "Apple Development",
		"Apple Development: jane@example.com (BBBB)": "Apple Development",
		"Mac Developer: Jane Doe (OLD)":              "Mac Developer",
	} {
		if got := identityKind(in); got != want {
			t.Errorf("identityKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSigningTeam(t *testing.T) {
	for in, want := range map[string]string{
		"Developer ID Application: Jane Doe (AAAA)": "AAAA",
		"Apple Development: Jane (Work) (BBBB)":     "BBBB",
		"-":                                         "",
		"":                                          "",
		"0000000000000000000000000000000000000001": "",
	} {
		if got := signingTeam(in); got != want {
			t.Errorf("signingTeam(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCertificateTeams(t *testing.T) {
	_, certs, hashes := testKeychain(t, []string{otherDev, teamDev, "No Team"}, map[string]string{otherDev: "OTHER67890", teamDev: "TEAM123456"})
	certs = append([]byte("-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n"), certs...)
	got := certificateTeams(certs)
	want := map[string]string{hashes[otherDev]: "OTHER67890", hashes[teamDev]: "TEAM123456"}
	if len(got) != len(want) {
		t.Errorf("certificateTeams = %v, want %v", got, want)
	}
	for hash, team := range want {
		if got[hash] != team {
			t.Errorf("certificateTeams[%s] = %q, want %q", hash, got[hash], team)
		}
	}
}

// An Apple Development identity's name ends in the user's identifier, so
// its team comes from its certificate.
func TestDevelopmentIdentityTeamFromCertificate(t *testing.T) {
	identities, certs, hashes := testKeychain(t,
		[]string{"Developer ID Application: Example (TEAM123456)", otherDev, teamDev},
		map[string]string{otherDev: "OTHER67890", teamDev: "TEAM123456"})
	teams := certificateTeams(certs)
	for _, tt := range []struct{ team, name string }{
		{"TEAM123456", teamDev},
		{"OTHER67890", otherDev},
		{"NOPE000000", otherDev},
		{"", otherDev},
	} {
		if hash, name := developmentIdentity(identities, teams, tt.team); hash != hashes[tt.name] || name != tt.name {
			t.Errorf("developmentIdentity(team %q) = %q, %q; want %q, %q", tt.team, hash, name, hashes[tt.name], tt.name)
		}
	}
}
