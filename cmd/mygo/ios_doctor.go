package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func runIOSDoctor(args []string) error {
	flags := newFlags("ios doctor", "", "Check local iOS build, signing, symbol and release tools. Does not create certificates or log in.")
	release := flags.Bool("release", false, "require a local Apple Distribution identity with its private key")
	jsonOutput := flags.Bool("json", false, "machine-readable report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(flags.Args()) > 0 {
		return fmt.Errorf("unexpected arguments")
	}
	var checks []iosCheck
	add := func(name, detail string, err error) {
		if err != nil {
			detail = err.Error()
		}
		checks = append(checks, iosCheck{name, err == nil, detail})
	}
	if runtime.GOOS != "darwin" {
		add("macOS", "", fmt.Errorf("iOS builds need macOS and Xcode"))
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, name := range []string{"go", "xcodebuild", "codesign", "dwarfdump", "atos"} {
			p, err := exec.LookPath(name)
			add(name, p, err)
		}
		for _, sdk := range []string{"iphoneos", "iphonesimulator"} {
			data, err := exec.CommandContext(ctx, "xcrun", "--sdk", sdk, "--show-sdk-path").Output()
			if err == nil {
				_, err = os.Stat(strings.TrimSpace(string(data)))
			}
			add(sdk, strings.TrimSpace(string(data)), err)
		}
		for _, name := range []string{"devicectl", "simctl"} {
			data, err := exec.CommandContext(ctx, "xcrun", "--find", name).Output()
			add(name, strings.TrimSpace(string(data)), err)
		}
		data, err := exec.CommandContext(ctx, "security", "find-identity", "-v", "-p", "codesigning").Output()
		development := strings.Count(string(data), "Apple Development:") + strings.Count(string(data), "iPhone Developer:")
		distribution := strings.Count(string(data), "Apple Distribution:") + strings.Count(string(data), "iPhone Distribution:")
		if *release && distribution == 0 {
			err = fmt.Errorf("no Apple Distribution identity with private key found")
		}
		add("signing", fmt.Sprintf("%d development / %d distribution identities", development, distribution), err)
	}
	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(checks); err != nil {
			return err
		}
	} else {
		for _, c := range checks {
			mark := "PASS"
			if !c.Passed {
				mark = "FAIL"
			}
			fmt.Printf("%s %-16s %s\n", mark, c.Name, c.Detail)
		}
	}
	for _, c := range checks {
		if !c.Passed {
			return fmt.Errorf("iOS toolchain checks failed")
		}
	}
	return nil
}
