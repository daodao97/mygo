package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func backgroundConfig() Config {
	return Config{Identifier: "org.example.app", IOS: IOS{BackgroundTasks: []IOSBackgroundTask{{"org.example.app.refresh", "PrepareRefresh", "RunRefresh", "CancelRefresh", "NextRefresh"}}}}
}
func TestIOSBackgroundGeneration(t *testing.T) {
	c := backgroundConfig()
	if e := validateIOSBackground(&c); e != nil {
		t.Fatal(e)
	}
	home := t.TempDir()
	targets, e := iosPrepareNative(&c, home, nil)
	if e != nil {
		t.Fatal(e)
	}
	main, _ := os.ReadFile(filepath.Join(home, "main.m"))
	if !strings.Contains(string(main), "MyGoRegisterConfiguredBackgroundTasks(); }") {
		t.Fatal(string(main))
	}
	source, _ := os.ReadFile(filepath.Join(home, "Native/MyGoApp/Background/MyGoConfiguredBackgroundTasks.m"))
	if !strings.Contains(string(source), `MyGoRegisterBackgroundRefresh("org.example.app.refresh", PrepareRefresh, RunRefresh, CancelRefresh, NextRefresh)`) {
		t.Fatal(string(source))
	}
	project := iosNativeProject(iosProject(&c, "15.0", ""), &c, "", targets)
	if !strings.Contains(project, "MyGoBackgroundRefresh.m") || !strings.Contains(project, "BackgroundTasks") {
		t.Fatal("runtime not linked")
	}
	plist := string(iosInfoPlist(&c, "15.0"))
	for _, want := range []string{"BGTaskSchedulerPermittedIdentifiers", "org.example.app.refresh", "UIBackgroundModes", "fetch"} {
		if !strings.Contains(plist, want) {
			t.Fatal("missing", want)
		}
	}
	plain := Config{Identifier: "org.example.app"}
	d := string(iosInfoPlist(&plain, "15.0"))
	if strings.Contains(d, "BGTaskScheduler") {
		t.Fatal("unused feature enabled")
	}
}
func TestIOSBackgroundValidation(t *testing.T) {
	for _, mutate := range []func(*Config){func(c *Config) { c.IOS.BackgroundTasks[0].Run = "invalid();" }, func(c *Config) { c.IOS.BackgroundTasks[0].Identifier = "org.other.refresh" }, func(c *Config) { c.IOS.Native.EntryPoint = "main.m" }, func(c *Config) { c.IOS.BackgroundTasks = append(c.IOS.BackgroundTasks, c.IOS.BackgroundTasks[0]) }, func(c *Config) {
		c.IOS.InfoPlist = map[string]any{"BGTaskSchedulerPermittedIdentifiers": []string{"old"}}
	}, func(c *Config) { c.IOS.BackgroundTasks[0].NextDelay = c.IOS.BackgroundTasks[0].Run }} {
		c := backgroundConfig()
		mutate(&c)
		if e := validateIOSBackground(&c); e == nil {
			t.Fatal("invalid declaration accepted", c.IOS.BackgroundTasks)
		}
	}
}
