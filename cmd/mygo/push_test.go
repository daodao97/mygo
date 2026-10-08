package main

import (
	"github.com/egoist/mygo/push/apns"
	"strings"
	"testing"
)

func TestPushSetupInfersProjectIdentityAndKey(t *testing.T) {
	c := &Config{Identifier: "dev.newproject.app", IOS: IOS{DevelopmentTeam: "TEAM123456", PushNotifications: true}}
	cfg, err := pushSetupConfig(c, "/private/AuthKey_KEY1234567.p8", "", "", "")
	if err != nil || cfg.Topic != c.Identifier || cfg.TeamID != c.IOS.DevelopmentTeam || cfg.KeyID != "KEY1234567" || cfg.Environment != apns.Sandbox {
		t.Fatal(cfg, err)
	}
	if c.IOS.Entitlements != nil {
		t.Fatal("setup modified source configuration")
	}
	if !strings.Contains(string(iosEntitlements(c)), "development") {
		t.Fatal("push toggle omitted signing capability")
	}
	c.IOS.Entitlements = map[string]any{"aps-environment": "production"}
	cfg, err = pushSetupConfig(c, "renamed.p8", "KEY1234567", "TEAM987654", "")
	if err != nil || cfg.Environment != apns.Production || cfg.TeamID != "TEAM987654" || !strings.Contains(string(iosEntitlements(c)), "production") {
		t.Fatal("explicit signing environment or key overrides ignored", cfg, err)
	}
	cfg, err = pushSetupConfig(c, "AuthKey_KEY1234567.p8", "", "", "sandbox")
	if err != nil || cfg.Environment != apns.Sandbox {
		t.Fatal("sender environment override ignored", cfg, err)
	}
}

func TestPushSetupRequiresCapabilityAndRecognizableKey(t *testing.T) {
	c := &Config{}
	if _, err := pushSetupConfig(c, "AuthKey_KEY1234567.p8", "", "", ""); err == nil {
		t.Fatal("disabled push capability accepted")
	}
	c.IOS.PushNotifications = true
	if _, err := pushSetupConfig(c, "key.p8", "", "", ""); err == nil {
		t.Fatal("missing key identifier accepted")
	}
	if _, err := readPushKey(strings.NewReader(strings.Repeat("x", 8193))); err == nil {
		t.Fatal("unbounded key file accepted")
	}
}
