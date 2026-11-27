package main

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"regexp"
	"strings"
)

var identityLine = regexp.MustCompile(`^\s*\d+\)\s+([0-9A-Fa-f]{40})\s+"(.*)"`)

// developmentIdentity returns the hash and name of the identity that signs
// the development app when mygo dev has no -sign, from the output of
// security find-identity -v -p codesigning: an Apple Development identity,
// as Xcode signs Debug builds, or one of its old name, Mac Developer. When
// there are several, as for several teams, the one of team (that of
// macos.signingIdentity), else the first. An identity is of team when its
// name ends in it, or when teams, the teams of the keychains' certificates
// by hash (certificateTeams), says so: an Apple Development identity's name
// ends in the user's own identifier, not the team's. Without one, it
// returns "".
//
// Its signature, unlike an ad hoc one, is the same from build to build: the
// designated requirement names the identifier and the certificate, not a
// hash of the code. macOS keeps what the user allowed the app, such as
// Accessibility or Screen Recording, and "Always Allow" for its Keychain
// items, by that requirement, so with ad hoc signing it takes every build
// for another app, asking again.
func developmentIdentity(out string, teams map[string]string, team string) (hash, name string) {
	var first [2]string
	for _, prefix := range []string{"Apple Development: ", "Mac Developer: "} {
		for line := range strings.SplitSeq(out, "\n") {
			m := identityLine.FindStringSubmatch(line)
			if m == nil || !strings.HasPrefix(m[2], prefix) {
				continue
			}
			if team != "" && (strings.HasSuffix(m[2], "("+team+")") || teams[strings.ToUpper(m[1])] == team) {
				return m[1], m[2]
			}
			if first[0] == "" {
				first = [2]string{m[1], m[2]}
			}
		}
	}
	return first[0], first[1]
}

// certificateTeams returns the team of each certificate in data, the output
// of security find-certificate -a -p, by its SHA-1 hash in upper-case hex,
// as find-identity prints it. A certificate's team is its subject's
// organizational unit.
func certificateTeams(data []byte) map[string]string {
	teams := map[string]string{}
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return teams
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || len(cert.Subject.OrganizationalUnit) == 0 {
			continue
		}
		sum := sha1.Sum(block.Bytes)
		teams[strings.ToUpper(hex.EncodeToString(sum[:]))] = cert.Subject.OrganizationalUnit[0]
	}
}

// signingTeam returns the team identifier at the end of an identity's
// name, as TEAMID of "Developer ID Application: Jane Doe (TEAMID)", or "".
func signingTeam(identity string) string {
	if !strings.HasSuffix(identity, ")") {
		return ""
	}
	i := strings.LastIndex(identity, "(")
	if i < 0 {
		return ""
	}
	return identity[i+1 : len(identity)-1]
}

// identityKind returns the kind of an identity, as "Apple Development" of
// "Apple Development: Jane Doe (AAAA)": logs show it instead of the name and
// team, which they can be shared with.
func identityKind(name string) string {
	kind, _, _ := strings.Cut(name, ": ")
	return kind
}
