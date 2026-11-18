//go:build windows && (amd64 || arm64)

package windows

import (
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// TestOpenDialogOptions checks the options of open dialogs: a folder
// picker never forces items of the file system, which freezes it on WSL
// shares (#2024), even when the dialog has it by default; a file picker
// still does.
func TestOpenDialogOptions(t *testing.T) {
	const fosNoChangeDir = 0x8
	for _, tc := range []struct {
		name    string
		current uint32
		o       platform.OpenDialogOptions
		want    uint32
	}{
		{"files", 0, platform.OpenDialogOptions{OpenFiles: true}, fosForceFileSystem | fosPathMustExist | fosFileMustExist},
		{"files, several, hidden", fosNoChangeDir, platform.OpenDialogOptions{OpenFiles: true, Multiple: true, ShowHiddenFiles: true},
			fosNoChangeDir | fosForceFileSystem | fosPathMustExist | fosFileMustExist | fosAllowMultiselect | fosForceShowHidden},
		{"folders", 0, platform.OpenDialogOptions{OpenDirectories: true}, fosPickFolders | fosPathMustExist},
		{"folders, forced by default", fosNoChangeDir | fosForceFileSystem, platform.OpenDialogOptions{OpenDirectories: true},
			fosNoChangeDir | fosPickFolders | fosPathMustExist},
		{"folders, several", 0, platform.OpenDialogOptions{OpenDirectories: true, Multiple: true}, fosPickFolders | fosPathMustExist | fosAllowMultiselect},
	} {
		if got := openDialogOptions(tc.current, &tc.o); got != tc.want {
			t.Errorf("%s: options %#x, want %#x", tc.name, got, tc.want)
		}
	}
}

// TestFileSystemPaths checks that items without a path, such as This PC
// in a folder picker, are left out, and that a choice of only those
// counts as canceled.
func TestFileSystemPaths(t *testing.T) {
	for _, tc := range []struct {
		in, want []string
	}{
		{nil, nil},
		{[]string{""}, nil},
		{[]string{"", ""}, nil},
		{[]string{`C:\a`}, []string{`C:\a`}},
		{[]string{`C:\a`, "", `\\wsl.localhost\Ubuntu\home\b`}, []string{`C:\a`, `\\wsl.localhost\Ubuntu\home\b`}},
	} {
		got := fileSystemPaths(tc.in)
		if !slices.Equal(got, tc.want) || (tc.want == nil) != (got == nil) {
			t.Errorf("fileSystemPaths(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

// TestRefused checks that a folder picker refuses a choice with an item
// that is not a folder of the file system, as This PC, and takes the
// others.
func TestRefused(t *testing.T) {
	for _, tc := range []struct {
		paths []string
		want  bool
	}{
		{nil, false},
		{[]string{`C:\a`}, false},
		{[]string{`C:\a`, `\\wsl.localhost\Ubuntu\home\b`}, false},
		{[]string{""}, true},
		{[]string{`C:\a`, ""}, true},
	} {
		if got := refused(tc.paths); got != tc.want {
			t.Errorf("refused(%q) = %v, want %v", tc.paths, got, tc.want)
		}
	}
}

// TestFolderCheck checks the events of a folder picker as the dialog
// sees them: it takes the check as IFileDialogEvents, OnFileOk accepts
// when there are no results to refuse, and the check is gone once the
// dialog let it go.
func TestFolderCheck(t *testing.T) {
	comInit(t)
	dlg := createFileDialog(&clsidFileOpenDialog, &iidIFileOpenDialog)
	if dlg == 0 {
		t.Fatal("no IFileOpenDialog")
	}
	before := len(folderChecks)
	unadvise := dialogs{}.checkFolders(dlg)
	if len(folderChecks) != before+1 {
		t.Fatalf("%d checks after Advise, want %d", len(folderChecks), before+1)
	}
	var p uintptr
	for k, c := range folderChecks {
		if c.dlg == dlg {
			p = k
		}
	}
	if p == 0 || folderChecks[p].refs < 1 {
		t.Fatal("the dialog holds no reference to its check")
	}
	if q := queryInterface(p, &iidIFileDialogEvents); q != p {
		t.Errorf("QueryInterface(IFileDialogEvents) = %#x, want %#x", q, p)
	} else {
		release(q)
	}
	if q := queryInterface(p, &iidIMarshal); q != 0 {
		t.Errorf("QueryInterface(IMarshal) = %#x, want none", q)
		release(q)
	}
	// Outside OnFileOk of a shown dialog there are no results: accepted,
	// and fileSystemPaths makes a cancel of it.
	if hr := comCall(p, 3, dlg); hr != sOK {
		t.Errorf("OnFileOk = %#x, want S_OK", hr)
	}
	for i := 4; i <= 6; i++ {
		if hr := comCall(p, i, dlg, 0); hr != sOK {
			t.Errorf("method %d = %#x, want S_OK", i, hr)
		}
	}
	unadvise()
	release(dlg)
	if len(folderChecks) != before {
		t.Errorf("%d checks after Unadvise, want %d", len(folderChecks), before)
	}
}

// TestLibraryFolder checks that a library stands for the folder it saves
// to, and that other items without a path, as This PC, have none.
func TestLibraryFolder(t *testing.T) {
	comInit(t)
	parse := func(name string) uintptr {
		t.Helper()
		var item uintptr
		if r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(u16(name))), 0, uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item))); failed(r) || item == 0 {
			t.Fatalf("%s: %#x", name, r)
		}
		return item
	}
	thisPC := parse("::{20D04FE0-3AEA-1069-A2D8-08002B30309D}")
	if p := itemPath(thisPC); p != "" {
		t.Errorf("This PC has the path %q", p)
	}
	if p := libraryFolder(thisPC); p != "" {
		t.Errorf("This PC is a library of %q", p)
	}
	release(thisPC)

	dir := t.TempDir()
	folder := parse(dir)
	defer release(folder)
	if p := libraryFolder(folder); p != "" {
		t.Errorf("a folder is a library of %q", p)
	}
	// A library of that folder, saved beside it.
	const (
		slAddFolder            = 5
		slSetDefaultSaveFolder = 10
		slSave                 = 18
		lsfMakeUniqueName      = 2
	)
	lib, err := coCreate(clsidShellLibrary, iidIShellLibrary)
	if err != nil {
		t.Fatal(err)
	}
	defer release(lib)
	if hr := comCall(lib, slAddFolder, folder); failed(hr) {
		t.Fatalf("AddFolder: %#x", hr)
	}
	if hr := comCall(lib, slSetDefaultSaveFolder, dsftDetect, folder); failed(hr) {
		t.Fatalf("SetDefaultSaveFolder: %#x", hr)
	}
	var saved uintptr
	if hr := comCall(lib, slSave, folder, uintptr(unsafe.Pointer(u16("Test"))), lsfMakeUniqueName, uintptr(unsafe.Pointer(&saved))); failed(hr) || saved == 0 {
		t.Fatalf("Save: %#x", hr)
	}
	defer release(saved)
	// The folder's own path, which is long where TEMP is short (8.3).
	if got, want := libraryFolder(saved), itemPath(folder); want == "" || !strings.EqualFold(got, want) {
		t.Errorf("the library saves to %q, want %q", got, want)
	}
}
