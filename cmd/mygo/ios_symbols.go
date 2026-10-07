package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

type iosSymbolEvidence struct {
	UUIDs      map[string]string `json:"uuids"`
	Function   string            `json:"function"`
	Source     string            `json:"source"`
	Address    string            `json:"address"`
	Symbolized string            `json:"symbolized"`
}

var iosUUIDPattern = regexp.MustCompile(`(?m)^UUID: ([A-Fa-f0-9-]{36}) \(([^)]+)\)`)

func iosUUIDs(path string) (map[string]string, error) {
	out, err := exec.Command("xcrun", "dwarfdump", "--uuid", path).Output()
	if err != nil {
		return nil, fmt.Errorf("reading Mach-O UUIDs: %w", err)
	}
	result := map[string]string{}
	for _, row := range iosUUIDPattern.FindAllStringSubmatch(string(out), -1) {
		result[row[2]] = strings.ToUpper(row[1])
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no Mach-O UUID in %s", path)
	}
	return result, nil
}

func iosVerifySymbols(binary, bundle string) (iosSymbolEvidence, error) {
	e := iosSymbolEvidence{}
	var err error
	e.UUIDs, err = iosUUIDs(binary)
	if err != nil {
		return e, err
	}
	dwarf := filepath.Join(bundle, "Contents", "Resources", "DWARF", filepath.Base(binary))
	symbolUUIDs, err := iosUUIDs(dwarf)
	if err != nil {
		return e, err
	}
	if !reflect.DeepEqual(e.UUIDs, symbolUUIDs) {
		return e, fmt.Errorf("dSYM UUIDs do not match the application: app=%v symbols=%v", e.UUIDs, symbolUUIDs)
	}
	if len(e.UUIDs) != 1 || e.UUIDs["arm64"] == "" {
		return e, fmt.Errorf("expected an arm64 iOS binary")
	}
	for _, function := range []string{"main.main", "runtime.gopanic"} {
		out, err := exec.Command("xcrun", "dwarfdump", "--name="+function, dwarf).Output()
		if err != nil {
			return e, fmt.Errorf("reading Go DWARF: %w", err)
		}
		source := regexp.MustCompile(`DW_AT_decl_file\s*\("([^"\n]+\.go)"\)`).FindSubmatch(out)
		address := regexp.MustCompile(`DW_AT_low_pc\s*\((0x[0-9a-fA-F]+)\)`).FindSubmatch(out)
		if source == nil || address == nil {
			continue
		}
		e.Function, e.Source, e.Address = function, string(source[1]), string(address[1])
		out, err = exec.Command("xcrun", "atos", "-arch", "arm64", "-o", dwarf, e.Address).Output()
		if err != nil {
			return e, fmt.Errorf("symbolizing Go code: %w", err)
		}
		e.Symbolized = strings.TrimSpace(string(out))
		if !strings.Contains(e.Symbolized, ".go:") || !strings.Contains(e.Symbolized, function) {
			return e, fmt.Errorf("Go function could not be resolved to a source line: %s", e.Symbolized)
		}
		return e, nil
	}
	return e, fmt.Errorf("dSYM lacks Go source/function debug information; do not build the Go archive with -s/-w")
}
