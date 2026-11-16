//go:build windows && (amd64 || arm64)

package windows

import (
	"strings"
	"unsafe"
)

// Vtable indices of the WebView2 interfaces used here, from WebView2.h
// (SDK 1.0.4191.47). Each interface extends the previous version, so the
// indices stay valid for newer runtimes.
const (
	// ICoreWebView2Environment
	envCreateController          = 3
	envCreateWebResourceResponse = 4

	// ICoreWebView2Controller, ICoreWebView2Controller2
	ctlPutIsVisible                      = 4
	ctlPutBounds                         = 6
	ctlGetZoomFactor                     = 7
	ctlPutZoomFactor                     = 8
	ctlMoveFocus                         = 12
	ctlAddAcceleratorKeyPressed          = 19
	ctlNotifyParentWindowPositionChanged = 23
	ctlClose                             = 24
	ctlGetCoreWebView2                   = 25
	ctl2PutDefaultBackgroundColor        = 27

	// ICoreWebView2
	wvGetSettings                            = 3
	wvGetSource                              = 4
	wvNavigate                               = 5
	wvNavigateToString                       = 6
	wvAddNavigationStarting                  = 7
	wvAddContentLoading                      = 9
	wvAddPermissionRequested                 = 23
	wvAddNavigationCompleted                 = 15
	wvAddProcessFailed                       = 25
	wvAddScriptToExecuteOnDocumentCreated    = 27
	wvRemoveScriptToExecuteOnDocumentCreated = 28
	wvExecuteScript                          = 29
	wvCapturePreview                         = 30
	wvReload                                 = 31
	wvAddWebMessageReceived                  = 34
	wvCallDevToolsProtocolMethod             = 36
	wvGetBrowserProcessId                    = 37
	wvGetCanGoBack                           = 38
	wvGetCanGoForward                        = 39
	wvGoBack                                 = 40
	wvGoForward                              = 41
	wvStop                                   = 43
	wvAddNewWindowRequested                  = 44
	wvAddDocumentTitleChanged                = 46
	wvGetDocumentTitle                       = 48
	wvOpenDevToolsWindow                     = 51
	wvAddWebResourceRequested                = 55
	wvAddWebResourceRequestedFilter          = 57
	wvRemoveWebResourceRequestedFilter       = 58
	wvAddWindowCloseRequested                = 59

	// ICoreWebView2Settings, ICoreWebView2Settings2
	setPutIsScriptEnabled                = 4
	setPutIsWebMessageEnabled            = 6
	setPutAreDefaultScriptDialogsEnabled = 8
	setPutIsStatusBarEnabled             = 10
	setPutAreDevToolsEnabled             = 12
	setPutAreDefaultContextMenusEnabled  = 14
	setPutIsZoomControlEnabled           = 18
	set2GetUserAgent                     = 21
	set2PutUserAgent                     = 22

	// ICoreWebView2Settings3
	set3GetAreBrowserAcceleratorKeysEnabled = 23
	set3PutAreBrowserAcceleratorKeysEnabled = 24

	// ICoreWebView2Settings5, ICoreWebView2Settings6
	set5GetIsPinchZoomEnabled       = 29
	set5PutIsPinchZoomEnabled       = 30
	set6GetIsSwipeNavigationEnabled = 31
	set6PutIsSwipeNavigationEnabled = 32

	// Event arguments and helpers.
	navStartingGetURI             = 3
	navStartingGetIsUserInitiated = 4
	navStartingPutCancel          = 8
	navStarting3GetNavigationKind = 12
	navCompletedGetIsSuccess      = 3
	navCompletedGetWebErrorStatus = 4
	msgTryGetWebMessageAsString   = 5
	msg2GetAdditionalObjects      = 6
	objectsGetCount               = 3
	objectsGetValueAtIndex        = 4
	fileGetPath                   = 3
	newWinGetURI                  = 3
	newWinPutNewWindow            = 4
	newWinPutHandled              = 6
	newWinGetIsUserInitiated      = 8 // after get_NewWindow (5), put_Handled (6), get_Handled (7)
	newWinGetDeferral             = 9
	newWinGetWindowFeatures       = 10
	featGetHasPosition            = 3
	featGetHasSize                = 4
	featGetLeft                   = 5
	featGetTop                    = 6
	featGetHeight                 = 7
	featGetWidth                  = 8
	resReqGetRequest              = 3
	resReqPutResponse             = 5
	resReqGetDeferral             = 6
	resReqGetResourceContext      = 7
	reqGetURI                     = 3
	reqGetMethod                  = 5
	reqGetContent                 = 7
	reqGetHeaders                 = 9
	headersRemoveHeader           = 7
	headersGetIterator            = 8
	iterGetCurrentHeader          = 3
	iterGetHasCurrentHeader       = 4
	iterMoveNext                  = 5
	deferralComplete              = 3
	procFailedGetKind             = 3
	wv4AddDownloadStarting        = 75  // ICoreWebView2_4
	wv13GetProfile                = 105 // ICoreWebView2_13
	profile2ClearBrowsingDataAll  = 12  // ICoreWebView2Profile2
	dlStartGetOperation           = 3
	dlStartPutCancel              = 5
	dlStartGetResultFilePath      = 6
	dlStartPutResultFilePath      = 7
	dlStartPutHandled             = 9
	dlOpAddStateChanged           = 7
	dlOpGetURI                    = 9
	dlOpGetState                  = 16
	dlOpGetInterruptReason        = 17
	permGetURI                    = 3
	permGetKind                   = 4
	permPutState                  = 7
	accelGetKeyEventKind          = 3
	accelGetVirtualKey            = 4
	accelPutHandled               = 8
)

// ICoreWebView2_8, whose methods follow ICoreWebView2_7's PrintToPdf (80)
// in WebView2.h: add_IsMutedChanged (81), remove_IsMutedChanged (82),
// get_IsMuted (83), put_IsMuted (84), add_IsDocumentPlayingAudioChanged
// (85), remove_IsDocumentPlayingAudioChanged (86) and
// get_IsDocumentPlayingAudio (87).
const (
	wv8AddIsMutedChanged                = 81
	wv8GetIsMuted                       = 83
	wv8PutIsMuted                       = 84
	wv8AddIsDocumentPlayingAudioChanged = 85
	wv8GetIsDocumentPlayingAudio        = 87
)

// ICoreWebView2_22 (SDK 1.0.2651.64, runtime 127) follows
// ICoreWebView2_21's methods in WebView2.h with
// AddWebResourceRequestedFilterWithRequestSourceKinds (123) and its Remove
// twin (124). Unlike ICoreWebView2's AddWebResourceRequestedFilter (57),
// whose filters reach only the main frame and its same-origin iframes,
// its filters reach iframes of other origins too.
const (
	wv22AddWebResourceRequestedFilterWithRequestSourceKinds = 123

	requestSourceDocument = 1 // COREWEBVIEW2_WEB_RESOURCE_REQUEST_SOURCE_KINDS_DOCUMENT
)

var (
	iidICoreWebView2Controller2                  = guid("c979903e-d4ca-4228-92eb-47ee3fa96eab")
	iidICoreWebView2Settings2                    = guid("ee9a0f68-f46c-4e32-ac23-ef8cac224d2a")
	iidICoreWebView2Settings3                    = guid("fdb5ab74-af33-4854-84f0-0a631deb5eba")
	iidICoreWebView2Settings5                    = guid("183e7052-1d03-43a0-ab99-98e043b66b39")
	iidICoreWebView2Settings6                    = guid("11cb3acd-9bc8-43b8-83bf-f40753714f87")
	iidICoreWebView2WebMessageReceivedEventArgs2 = guid("06fc7ab7-c90c-4297-9389-33ca01cf6d5e")
	iidICoreWebView2NavigationStartingEventArgs3 = guid("ddffe494-4942-4bd2-ab73-35b8ff40e19f")
	iidICoreWebView2File                         = guid("f2c19559-6bc1-4583-a757-90021be9afec")
	iidICoreWebView2_4                           = guid("20d02d59-6df2-42dc-bd06-f98a694b1302")
	iidICoreWebView2_8                           = guid("e9632730-6e1e-43ab-b7b8-7b2c9e62e094")
	iidICoreWebView2_13                          = guid("f75f09a8-667e-4983-88d6-c8773f315e84")
	iidICoreWebView2_22                          = guid("db75dfc7-a857-4632-a398-6969dde26c0a")
	iidICoreWebView2Profile2                     = guid("fa740d4b-5eae-4344-a8ad-74be31925397")
	iidICoreWebView2Environment8                 = guid("d6eb91dd-c3d2-45e5-bd29-6dc2bc4de9cf")
)

// ICoreWebView2Environment8's GetProcessInfos follows
// add_ProcessInfosChanged (16) and remove_ProcessInfosChanged (17); the
// collection it returns has get_Count and GetValueAtIndex, each
// ICoreWebView2ProcessInfo get_ProcessId and get_Kind.
const (
	env8GetProcessInfos      = 18
	procInfosGetCount        = 3
	procInfosGetValueAtIndex = 4
	procInfoGetProcessId     = 3
	procInfoGetKind          = 4

	processKindGPU = 4 // COREWEBVIEW2_PROCESS_KIND_GPU
)

const (
	hkeyCurrentUser  = 0x80000001
	hkeyLocalMachine = 0x80000002
	keyQueryValue    = 0x0001
)

// unthrottledArguments are the browser's arguments that keep the pages of
// hidden, minimized and covered windows running at full speed
// (PageOptions.BackgroundThrottling): timers unthrottled, renderers at
// the priority of visible ones, and covered windows not taken for hidden.
const unthrottledArguments = "--disable-background-timer-throttling --disable-renderer-backgrounding --disable-backgrounding-occluded-windows"

// autoplayArguments are the browser's arguments that let pages play
// audible media without a user gesture (PageOptions.Autoplay).
const autoplayArguments = "--autoplay-policy=no-user-gesture-required"

// browserArguments are the browser's additional arguments for a first page
// that asks not to be throttled, or to play without a user gesture, or
// both; "" for neither.
func browserArguments(unthrottled, autoplay bool) string {
	var args []string
	if unthrottled {
		args = append(args, unthrottledArguments)
	}
	if autoplay {
		args = append(args, autoplayArguments)
	}
	return strings.Join(args, " ")
}

// regDWORD reads a DWORD value, reporting whether it exists.
func regDWORD(root uintptr, path, name string) (uint32, bool) {
	var key uintptr
	if r, _, _ := procRegOpenKeyExW.Call(root, uintptr(unsafe.Pointer(u16(path))), 0, keyQueryValue, uintptr(unsafe.Pointer(&key))); r != 0 {
		return 0, false
	}
	defer procRegCloseKey.Call(key)
	var typ, v uint32
	size := uint32(4)
	if r, _, _ := procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(u16(name))), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&size))); r != 0 {
		return 0, false
	}
	return v, true
}
