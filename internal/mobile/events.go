package mobile

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
)

// handle handles an event of the host on the main thread and returns the
// reply the host waits for, if any.
func (b *Backend) handle(msg []byte) []byte {
	r := wire.Reader{Buf: msg}
	op := r.Uvarint()
	var w wire.Writer
	win := func(id int) *window { return b.windows[id] }
	switch op {
	case wire.EventReady:
		b.ready = true
		b.h.Ready()
	case wire.EventActive:
		var e wire.ActiveEvent
		e.Decode(&r)
		if e.Active {
			b.h.DidBecomeActive()
		} else {
			b.h.DidResignActive()
		}
	case wire.EventBackground:
		var e wire.BackgroundEvent
		e.Decode(&r)
		switch {
		case e.Background && !b.background:
			b.background = true
			b.h.PowerEvent("suspend")
		case !e.Background && b.background:
			// Coming back is the app opened again, as clicking its Dock
			// icon is on macOS; launching is not.
			b.background = false
			for _, w := range b.windows {
				if w.s != nil {
					w.s.foreground()
				}
			}
			// Started out of sight, as for a background task or by how iOS
			// says it launches, it's no coming back.
			if b.startedHidden {
				b.startedHidden = false
				break
			}
			b.h.PowerEvent("resume")
			if b.ready {
				b.h.Activated(b.front() != nil)
			}
		}
	case wire.EventThemeChanged:
		var t wire.ThemeReply
		b.call(&wire.Theme{}, &t)
		b.dark = t.Dark
		b.h.ThemeChanged()
		for _, w := range b.windows {
			if w.s != nil {
				w.s.themeChanged()
			}
		}
	case wire.EventLocaleChanged:
		b.forgetInfo()
		b.h.LocaleChanged()
	case wire.EventTimeZoneChanged:
		var e wire.TimeZoneChangedEvent
		e.Decode(&r)
		setTimeZone(e.ID)
		// What shows the time draws it again.
		b.h.LocaleChanged()
	case wire.EventOpenURLs:
		var e wire.OpenURLsEvent
		e.Decode(&r)
		var urls, files []string
		for _, u := range e.URLs {
			if len(u) > 7 && u[:7] == "file://" {
				files = append(files, u[7:])
			} else {
				urls = append(urls, u)
			}
		}
		if len(urls) > 0 {
			b.h.OpenURLs(urls)
		}
		if len(files) > 0 {
			b.h.OpenFiles(files)
		}
	case wire.EventMemoryWarning:
		b.h.MemoryWarning()
		for _, w := range b.windows {
			if w.s != nil {
				w.s.lowMemory()
			}
		}
		runtime.GC()
		debug.FreeOSMemory()
	case wire.EventTerminating:
		b.h.Terminating()

	case wire.EventResized:
		var e wire.ResizedEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.resized(&e)
		}
	case wire.EventBackPressed:
		var e wire.BackPressedEvent
		e.Decode(&r)
		reply := wire.BackPressedReply{}
		if w := win(e.ID); w != nil {
			reply.Handled = w.back()
		}
		reply.Encode(&w)
	case wire.EventMessage:
		var e wire.MessageEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.h.Message(e.Text)
		}
	case wire.EventWillNavigate:
		var e wire.WillNavigateEvent
		e.Decode(&r)
		reply := wire.WillNavigateReply{Allow: true}
		if win := win(e.ID); win != nil {
			reply.Allow = win.h.WillNavigate(platform.Navigation{URL: win.appURL(e.URL), IsMainFrame: e.MainFrame,
				UserInitiated: e.UserInitiated, Unasked: e.Unasked, Reload: e.Reload})
		}
		reply.Encode(&w)
	case wire.EventNewWindow:
		var e wire.NewWindowEvent
		e.Decode(&r)
		reply := wire.NewWindowReply{}
		if win := win(e.ID); win != nil {
			nw := win.h.NewWindow(platform.NewWindowRequest{URL: win.appURL(e.URL), UserInitiated: e.UserInitiated, Disposition: "new-window"})
			if nw, ok := nw.(*window); ok && nw != nil {
				// The app's window takes the page: phones show it in front.
				nw.LoadURL(win.appURL(e.URL))
				reply.Handled = true
			}
		}
		reply.Encode(&w)
	case wire.EventPageState:
		var e wire.PageStateEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.url, w.loading, w.canBack, w.canFwd = w.appURL(e.URL), e.Loading, e.CanGoBack, e.CanGoForward
			if e.Title != w.pageTitle {
				w.pageTitle = e.Title
				w.h.TitleChanged(e.Title)
			}
		}
	case wire.EventNavigationStarted:
		var e wire.NavigationStartedEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.loading = true
			w.h.NavigationStarted(w.appURL(e.URL))
		}
	case wire.EventNavigationCommitted:
		var e wire.NavigationCommittedEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.url = w.appURL(e.URL)
			w.h.NavigationCommitted(w.url)
		}
	case wire.EventLoadFinished:
		var e wire.LoadFinishedEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.loading = false
			w.h.LoadFinished()
		}
	case wire.EventLoadFailed:
		var e wire.LoadFailedEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.loading = false
			w.h.LoadFailed(w.appURL(e.URL), e.Code, e.Description)
		}
	case wire.EventWindowFront:
		var e wire.WindowFrontEvent
		e.Decode(&r)
		old := b.front()
		var w *window
		if e.ID != 0 {
			w = win(e.ID)
		} else {
			// The app's first scene: the last shown there.
			for _, s := range b.shown {
				if !s.opts.OwnScene {
					w = s
				}
			}
		}
		if w != nil && w.visible {
			b.unshow(w)
			b.shown = append(b.shown, w)
			b.refocus(old)
		}
	case wire.EventClosedByPage:
		var e wire.ClosedByPageEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.h.ClosedByPage()
		}
	case wire.EventProcessGone:
		var e wire.ProcessGoneEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.h.RenderProcessGone(e.Reason)
		}
	case wire.EventPermissionRequested:
		var e wire.PermissionRequestedEvent
		e.Decode(&r)
		reply := wire.PermissionRequestedReply{}
		if w := win(e.ID); w != nil {
			reply.Allow = w.h.PermissionRequested(e.Kinds, w.appURL(e.Origin))
		}
		reply.Encode(&w)
	case wire.EventSchemeRequest:
		var e wire.SchemeRequestEvent
		e.Decode(&r)
		b.schemeRequest(&e)
	case wire.EventSchemeCancel:
		var e wire.SchemeCancelEvent
		e.Decode(&r)
		if s := b.schemes[e.Req]; s != nil {
			delete(b.schemes, e.Req)
			s.cancel()
		}
	case wire.EventPluginEvent:
		var e wire.PluginEventEvent
		e.Decode(&r)
		if b.pluginEvents != nil {
			b.pluginEvents(e.Plugin, e.Name, []byte(e.Data))
		}
	case wire.EventNetworkChanged:
		var e wire.NetworkChangedEvent
		e.Decode(&r)
		b.h.NetworkChanged(platform.NetworkStatus{Online: e.Online, Metered: e.Metered})
	case wire.EventAsyncResult, wire.EventCaptured, wire.EventMessageBoxResult, wire.EventFilesResult, wire.EventNotifyDone,
		wire.EventShareResult, wire.EventMenuPicked, wire.EventPluginResult:
		// Answers to a request: its number first, then the rest.
		rest := r
		req := r.Int()
		b.answer(req, rest)
	case wire.EventNotificationClicked:
		var e wire.NotificationClickedEvent
		e.Decode(&r)
		b.h.NotificationClicked(e.NotificationID)
	default:
		if !b.handleNavigation(op, &r) {
			b.handleSurface(op, &r, &w)
		}
	}
	if !wire.EventWaits(op) {
		return nil
	}
	if w.Buf == nil {
		w.Buf = []byte{}
	}
	return w.Buf
}

// back handles Android's back for the window: its page goes back if it
// can, else the app's handlers may take it.
func (w *window) back() bool {
	if w.s != nil {
		return w.s.back()
	}
	if w.canBack {
		w.GoBack()
		return true
	}
	return false
}

// schemeRequest is a request of one of the app's schemes, which the core
// answers through it.
type schemeRequest struct {
	b      *Backend
	req    int
	cancel context.CancelFunc
	done   bool
}

func (b *Backend) schemeRequest(e *wire.SchemeRequestEvent) {
	w := b.windows[e.ID]
	if w == nil {
		b.call(&wire.SchemeFail{Req: e.Req, Message: "no window"}, nil)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &schemeRequest{b: b, req: e.Req, cancel: cancel}
	b.schemes[e.Req] = s
	header := map[string][]string{}
	for i := 0; i+1 < len(e.Header); i += 2 {
		header[e.Header[i]] = append(header[e.Header[i]], e.Header[i+1])
	}
	var body io.ReadCloser
	if e.HasBody {
		body = io.NopCloser(bytes.NewReader(e.Body))
	}
	w.h.SchemeRequest(&platform.SchemeRequest{Context: ctx, Method: e.Method, URL: w.appURL(e.URL), Header: header, Body: body, Responder: s})
}

func (s *schemeRequest) Respond(status int, header map[string][]string) {
	var pairs []string
	for k, vs := range header {
		for _, v := range vs {
			pairs = append(pairs, k, v)
		}
	}
	s.b.call(&wire.SchemeRespond{Req: s.req, Status: status, Header: pairs}, nil)
}

func (s *schemeRequest) Write(p []byte) {
	if len(p) > 0 && !s.done {
		s.b.call(&wire.SchemeData{Req: s.req, Data: bytes.Clone(p)}, nil)
	}
}

func (s *schemeRequest) Finish() {
	if s.end() {
		s.b.call(&wire.SchemeFinish{Req: s.req}, nil)
	}
}

func (s *schemeRequest) Fail(err error) {
	if s.end() {
		s.b.call(&wire.SchemeFail{Req: s.req, Message: err.Error()}, nil)
	}
}

func (s *schemeRequest) end() bool {
	if s.done {
		return false
	}
	s.done = true
	delete(s.b.schemes, s.req)
	s.cancel()
	return true
}
