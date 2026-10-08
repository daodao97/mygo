package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/egoist/mygo/push/apns"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const pushUsage = `Usage: mygo push <setup|status> [flags] [arguments]

  setup [flags] AuthKey_KEYID.p8 [dir]  securely import an APNs key for the project
  status [flags] [dir]                verify the project's local sender configuration

Bundle ID and Team ID come from mygo.json or mygo.config.ts. Key ID is inferred
from Apple's downloaded filename. Metadata is stored in the application's user
data directory; the private key goes to macOS Keychain or an owner-only file.
No provider credentials are bundled in the iOS application. This command does
not create Apple Developer capabilities or keys.
`

func runPush(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(pushUsage)
		return nil
	}
	if args[0] != "setup" && args[0] != "status" {
		return errors.New("push command must be setup or status")
	}
	fs := newFlags("push "+args[0], "[flags] [arguments]", pushUsage)
	dataDir := fs.String("data-dir", "", "override application user-data directory (e.g. for a remote sender)")
	dev := fs.Bool("dev", false, "use the separate mygo dev application data directory")
	var keyID, teamID, environment *string
	if args[0] == "setup" {
		keyID = fs.String("key-id", "", "APNs key ID (default: AuthKey_KEYID.p8 filename)")
		teamID = fs.String("team", "", "Apple team ID (default: ios.developmentTeam)")
		environment = fs.String("environment", "", "sandbox or production (default: ios aps-environment, otherwise sandbox)")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	root, keyFile := ".", ""
	if args[0] == "setup" {
		if fs.NArg() < 1 || fs.NArg() > 2 {
			return errors.New("usage: mygo push setup [flags] AuthKey_KEYID.p8 [dir]")
		}
		keyFile = fs.Arg(0)
		if fs.NArg() == 2 {
			root = fs.Arg(1)
		}
	} else {
		if fs.NArg() > 1 {
			return flag.ErrHelp
		}
		if fs.NArg() == 1 {
			root = fs.Arg(0)
		}
	}
	c, err := loadConfig(root)
	if err != nil {
		return err
	}
	dir := *dataDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		name := c.Name
		if *dev {
			name += " Dev"
		}
		if filepath.Base(name) != name || name == "." || name == ".." {
			return errors.New("application name must be a directory basename; use -data-dir to override")
		}
		dir = filepath.Join(base, name)
	}
	if args[0] == "status" {
		p, err := apns.OpenProvider(dir)
		if err != nil {
			return err
		}
		if p.Topic() != c.Identifier {
			return errors.New("configured provider belongs to a different application")
		}
		fmt.Printf("APNs configured for %s\n", p.Topic())
		return nil
	}
	cfg, err := pushSetupConfig(c, keyFile, *keyID, *teamID, *environment)
	if err != nil {
		return err
	}
	// Bound reads: provider keys are small, and errors never print their contents.
	f, err := os.Open(keyFile)
	if err != nil {
		return errors.New("unable to open APNs key file")
	}
	defer f.Close()
	key, err := readPushKey(f)
	if err != nil {
		return err
	}
	if err := apns.ImportProvider(dir, cfg, key); err != nil {
		return err
	}
	fmt.Printf("APNs configured for %s (%s)\n", cfg.Topic, cfg.Environment)
	return nil
}

var downloadedPushKey = regexp.MustCompile(`^AuthKey_([A-Z0-9]{10})\.p8$`)

func readPushKey(r io.Reader) ([]byte, error) {
	key, err := io.ReadAll(io.LimitReader(r, 8193))
	if err != nil || len(key) > 8192 {
		return nil, errors.New("unable to read APNs key (maximum 8 KB)")
	}
	return key, nil
}

func pushSetupConfig(c *Config, file, keyID, teamID, environment string) (apns.ProviderConfig, error) {
	if !c.IOS.PushNotifications && c.IOS.Entitlements["aps-environment"] == nil {
		return apns.ProviderConfig{}, errors.New("enable ios.pushNotifications in the project configuration first")
	}
	if keyID == "" {
		if m := downloadedPushKey.FindStringSubmatch(filepath.Base(file)); m != nil {
			keyID = m[1]
		} else {
			return apns.ProviderConfig{}, errors.New("cannot infer Key ID; use Apple's AuthKey_KEYID.p8 filename or -key-id")
		}
	}
	if teamID == "" {
		teamID = c.IOS.DevelopmentTeam
	}
	if environment == "" {
		environment = "sandbox"
		if c.IOS.Entitlements["aps-environment"] == "production" {
			environment = "production"
		}
	}
	return apns.ProviderConfig{TeamID: teamID, KeyID: keyID, Topic: c.Identifier, Environment: apns.Environment(environment)}, nil
}
