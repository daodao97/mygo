package apns

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestActivityKitHTTP2ChannelAndProviderTopic(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		for key, want := range map[string]string{"apns-topic": "dev.example.app.push-type.liveactivity", "apns-push-type": "liveactivity", "apns-priority": "5"} {
			if got := r.Header.Get(key); got != want {
				t.Errorf("%s=%q, want %q", key, got, want)
			}
		}
		var wire map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Fatal(err)
		}
		if len(wire) != 1 || wire["aps"] == nil {
			t.Error("alert/custom metadata mixed into activity channel")
		}
		var aps map[string]json.RawMessage
		json.Unmarshal(wire["aps"], &aps)
		if string(aps["event"]) != `"update"` || string(aps["content-state"]) != `{"status":"running"}` || aps["alert"] != nil || aps["sound"] != nil {
			t.Error("invalid activity state", string(wire["aps"]))
		}
	})
	p := &Provider{client: c, topic: "dev.example.app"}
	n := testNotice()
	n.Topic = ""
	n.Payload = Payload{Title: "must not appear", LiveActivity: &LiveActivityPayload{Timestamp: 123, Event: "update", ContentState: json.RawMessage(`{"status":"running"}`), StaleDate: 456}}
	if _, err := p.Send(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	n.Topic = "other.app.push-type.liveactivity"
	if _, err := p.Send(context.Background(), n); err == nil {
		t.Fatal("provider allowed cross-app activity topic")
	}
}
func TestActivityPayloadRejectsUnstartableOrUnstructuredState(t *testing.T) {
	for _, p := range []LiveActivityPayload{{Timestamp: 1, Event: "start", ContentState: json.RawMessage(`{}`)}, {Timestamp: 1, Event: "update", ContentState: json.RawMessage(`null`)}, {Timestamp: 0, Event: "end", ContentState: json.RawMessage(`{}`)}} {
		if _, err := json.Marshal(Payload{LiveActivity: &p}); err == nil {
			t.Fatal("invalid activity payload accepted")
		}
	}
}
