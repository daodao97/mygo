package mygo

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
)

// OpenInAppBrowser presents the system browser without leaving the application.
// It returns when Done is pressed or ctx is canceled. iOS apps opt in with
// ios.native.support: ["browser"]. Other platforms return ErrUnsupported.
func OpenInAppBrowser(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("mygo: browser requires an HTTP(S) URL without credentials")
	}
	_, err = mobileExperienceRequest(ctx, "browser", raw)
	return err
}

// LiveActivityState is the fixed, public wire shape shared with the opt-in
// MyGoLiveActivityAttributes Swift helper and ActivityKit push content-state.
type LiveActivityState struct {
	Title     string   `json:"title"`
	Subtitle  string   `json:"subtitle"`
	Status    string   `json:"status"`
	URL       string   `json:"url"`
	Progress  *float64 `json:"progress"`
	UpdatedAt int64    `json:"updatedAt"`
}

type LiveActivityInfo struct {
	ID        string `json:"id"`
	Activity  string `json:"activity"`
	PushToken string `json:"pushToken"`
}

// UpdateLiveActivity starts or updates the activity identified by id. Native
// sources must opt into "live-activities" and the app must embed a WidgetKit
// extension using MyGoLiveActivityAttributes. Requires iOS 16.2 or newer.
func UpdateLiveActivity(ctx context.Context, id string, state LiveActivityState) (LiveActivityInfo, error) {
	if id == "" || len(id) > 256 || len(state.Title) > 512 || len(state.Subtitle) > 1024 || len(state.Status) > 128 || len(state.URL) > 1024 || state.Progress != nil && (math.IsNaN(*state.Progress) || math.IsInf(*state.Progress, 0) || *state.Progress < 0 || *state.Progress > 1) {
		return LiveActivityInfo{}, errors.New("mygo: invalid Live Activity state")
	}
	value, err := mobileExperienceRequest(ctx, "activity", struct {
		ID    string            `json:"id"`
		State LiveActivityState `json:"state"`
	}{id, state})
	var info LiveActivityInfo
	if err == nil {
		err = json.Unmarshal([]byte(value), &info)
	}
	return info, err
}

func EndLiveActivity(ctx context.Context, id string) error {
	if id == "" || len(id) > 256 {
		return errors.New("mygo: invalid Live Activity identifier")
	}
	_, err := mobileExperienceRequest(ctx, "activity-end", id)
	return err
}

// OnLiveActivityToken receives token rotation and activity dismissal on the
// main thread. A dismissed/ended activity has an empty PushToken. Tokens are
// private: forward them only to the authenticated service updating the activity.
func OnLiveActivityToken(fn func(LiveActivityInfo)) func() {
	if fn == nil {
		return func() {}
	}
	return hostLiveActivityToken(fn)
}

func mobileExperienceRequest(ctx context.Context, kind string, value any) (string, error) {
	needsApp("mobile experience")
	if ctx == nil {
		return "", errors.New("mygo: mobile experience requires a context")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	type result struct {
		value string
		err   error
	}
	ch := make(chan result, 1)
	finished := make(chan struct{})
	if !postMain(func() {
		cancel := hostMobileExperience(kind, string(data), func(value string, err error) {
			if ctx.Err() != nil {
				value, err = "", ctx.Err()
			}
			deliver(ch, result{value, err})
			close(finished)
		})
		go func() {
			select {
			case <-finished:
			case <-ctx.Done():
				postMain(cancel)
			}
		}()
	}) {
		return "", errLoopStopped
	}
	r := await(ch)
	return r.value, r.err
}
