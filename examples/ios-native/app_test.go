package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewAllFeaturesOpensCatalog(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"scrolled catalog", nil},
		{"detail", []string{"/features/dialogs"}},
		{"nested detail", []string{"/features/navigation", "/features/navigation/detail"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &demo{}
			s.initNavigation()
			for _, r := range s.app.Routers {
				r.Transition = ui.TransitionNone
			}
			for _, path := range tc.paths {
				s.app.Routers[1].Push(path)
			}
			s.app.Scroll[1].Y = 500
			s.app.Routers[2].Push("/status/lifecycle")
			s.app.NavigationValue = "Keep this draft"
			statusHistory := s.app.Routers[2].History()
			view := ui.NewTester(s.view, 375, 728)
			// An ordinary tab switch retains its selected detail/history.
			view.ClickAt(187, 695)
			wantPath := "/features"
			if len(tc.paths) > 0 {
				wantPath = tc.paths[len(tc.paths)-1]
			}
			if s.app.Routers[1].Path() != wantPath {
				t.Fatal("ordinary tab switch lost the remembered page")
			}
			view.ClickAt(62, 695)
			view.Scroll(8, 300, 0, 1000)
			button, ok := view.Find("View All Features")
			if !ok || button.Y < 54 || button.Y+button.H > 728-66 {
				t.Fatalf("catalog button is outside the page: %+v", button)
			}
			if err := view.Click("View All Features"); err != nil {
				t.Fatal(err)
			}
			if s.app.Tab != 1 || s.app.Routers[1].Path() != "/features" {
				t.Fatalf("catalog button opened tab %d, page %s", s.app.Tab, s.app.Routers[1].Path())
			}
			if !view.HasText("Feature Demos") || s.app.Routers[1].CanGoBack() || s.app.Scroll[1].Y != 0 {
				t.Fatalf("catalog did not open at its root/top: scroll %+v", s.app.Scroll[1])
			}
			if !reflect.DeepEqual(statusHistory, s.app.Routers[2].History()) || s.app.NavigationValue != "Keep this draft" {
				t.Fatal("catalog navigation changed another tab or the draft")
			}
		})
	}
}

func TestAppTabsAndRestoredHistory(t *testing.T) {
	s := &demo{}
	s.initNavigation()
	// This checks routing and saved state. Device tests exercise animated
	// transitions; do not depend on wall-clock animation progress here.
	for _, r := range s.app.Routers {
		r.Transition = ui.TransitionNone
	}
	view := ui.NewTester(s.view, 375, 728)
	for _, name := range []string{"Overview", "Features", "Status", "MyGo Mobile"} {
		if !view.HasText(name) {
			t.Fatalf("missing %q", name)
		}
	}
	if err := view.Click("Navigation & State"); err != nil {
		t.Fatal(err)
	}
	if s.app.Routers[0].Path() != "/home/navigation" {
		t.Fatal("navigation example not opened")
	}
	if err := view.Click("Open Detail"); err != nil {
		t.Fatal(err)
	}
	if s.app.Routers[0].Path() != "/home/navigation/detail" {
		t.Fatal("nested demo did not open")
	}
	s.app.NavigationValue = "中文草稿 👋"
	view.ClickAt(187, 695) // bottom feature tab
	if s.app.Tab != 1 {
		t.Fatal("bottom tab did not change")
	}
	if err := view.Click("Dialogs & Sharing"); err != nil {
		t.Fatal(err)
	}
	if s.app.Routers[1].Path() != "/features/dialogs" {
		t.Fatal("dialog example not opened")
	}
	view.ClickAt(62, 695)
	if s.app.Tab != 0 || s.app.Routers[0].Path() != "/home/navigation/detail" {
		t.Fatal("tab lost its navigation stack")
	}
	if err := view.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if s.app.Routers[0].Path() != "/home/navigation" {
		t.Fatal("back did not return to navigation example")
	}
	// Routers restore directly from JSON, including their nested history.
	// The demo does not replay paths or maintain a second history.
	s.app.Tab = 0
	s.app.Routers[0].Push("/home/navigation/detail")
	data, err := json.Marshal(s.app)
	if err != nil {
		t.Fatal(err)
	}
	restored := &demo{}
	if err := json.Unmarshal(data, &restored.app); err != nil {
		t.Fatal(err)
	}
	restored.initNavigation()
	if restored.app.Routers[0].Path() != "/home/navigation/detail" {
		t.Fatal("nested page was not restored")
	}
	if restored.app.NavigationValue != "中文草稿 👋" {
		t.Fatal("draft was not restored")
	}
	restored.app.Routers[0].Pop()
	if restored.app.Routers[0].Path() != "/home/navigation" {
		t.Fatal("restored page cannot go back")
	}
	restored.app.Routers[0].Pop()
	if restored.app.Routers[0].Path() != "/home" || restored.app.Routers[0].CanGoBack() {
		t.Fatal("restored root has incorrect history")
	}
}

func TestAppLegacyHistoryCheckpoint(t *testing.T) {
	var state appState
	if err := json.Unmarshal([]byte(`{"Tab":1,"History":[["/home"],["/features","/features/navigation","/features/navigation/detail"],["/status"]],"NavigationValue":"中文草稿 👋"}`), &state); err != nil {
		t.Fatal(err)
	}
	s := &demo{app: state}
	s.initNavigation()
	if s.app.Routers[1].Path() != "/features/navigation/detail" || s.app.NavigationValue != "中文草稿 👋" {
		t.Fatal("upgrade lost navigation or editor state")
	}
	s.app.Routers[1].Pop()
	data, err := json.Marshal(s.app)
	if err != nil {
		t.Fatal(err)
	}
	var restored appState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Routers[1].Path() != "/features/navigation" || !restored.Routers[1].CanGoForward() {
		t.Fatal("new checkpoint lost the current index or forward history")
	}
	restored.Routers[1].Forward()
	if restored.Routers[1].Path() != "/features/navigation/detail" {
		t.Fatal("forward history did not survive relaunch")
	}
}

func TestDiagnosticEntryIsNotRestored(t *testing.T) {
	s := &demo{}
	s.initNavigation()
	s.app.Tab = 1
	s.app.Routers[1].Push("/features/images")
	s.app.Diagnostics = true
	data, err := json.Marshal(s.app)
	if err != nil {
		t.Fatal(err)
	}
	var restored demo
	if err := json.Unmarshal(data, &restored.app); err != nil {
		t.Fatal(err)
	}
	if restored.app.Diagnostics || restored.app.Tab != 1 || restored.app.Routers[1].Path() != "/features/images" {
		t.Fatal("diagnostic mode restored or feature navigation lost")
	}
	// Old test checkpoints must also return to the feature app.
	if err := json.Unmarshal([]byte(`{"Diagnostics":true}`), &restored.app); err != nil {
		t.Fatal(err)
	}
	view := ui.NewTester(restored.view, 375, 728)
	for _, label := range []string{"Overview", "Features", "Status", "Raster Images"} {
		if _, ok := view.Find(label); !ok {
			t.Fatalf("ordinary launch missing %q", label)
		}
	}
}
