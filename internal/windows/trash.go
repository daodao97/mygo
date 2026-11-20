//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// The recycle bin, through IFileOperation as Electron's shell.trashItem.
// Windows deletes an item for good when it cannot recycle it, and without
// a UI nothing asks first. On a drive without a recycle bin, as network
// shares and many removable drives are, or with one that removes files
// immediately, PreDeleteItem comes without TSF_DELETE_RECYCLE_IF_POSSIBLE,
// and a progress sink refuses the delete, so TrashItem fails instead. A
// folder too big for the recycle bin ends there too: Windows deletes its
// items one by one, without the flag. A file too big comes with the flag
// and is deleted all the same, so TrashItem compares its size first.
// Windows reads the size once per process, so changing it directly in the
// registry may still let a file be deleted for good. PostDeleteItem then
// has no new item, and TrashItem fails saying Windows deleted it.

var (
	clsidFileOperation            = guid("3ad05575-8857-4850-9277-11b85bdb8e09")
	iidIFileOperation             = guid("947aab5f-0a5c-4c13-b4d6-4bf7836fc9f8")
	iidIFileOperationProgressSink = guid("04b0f1a7-9490-44bc-96e1-4296a31252e2")
	trashSinkVtbl                 [19]uintptr
	trashSinkOnce                 sync.Once
	trashSinks                    = map[uintptr]*trashSink{}
)

const (
	fofSilent                  = 0x0004
	fofNoConfirmation          = 0x0010
	fofAllowUndo               = 0x0040
	fofNoConfirmMkdir          = 0x0200
	fofNoErrorUI               = 0x0400
	fofNoUI                    = fofSilent | fofNoConfirmation | fofNoErrorUI | fofNoConfirmMkdir
	fofxRecycleOnDelete        = 0x00080000
	fofxAddUndoRecord          = 0x20000000
	tsfDeleteRecycleIfPossible = 0x80
	copyEngineSNotHandled      = 0x00270003 // COPYENGINE_S_NOT_HANDLED
	eAbort                     = 0x80004004

	// IFileOperation methods.
	foSetOperationFlags       = 5
	foDeleteItem              = 18
	foPerformOperations       = 21
	foGetAnyOperationsAborted = 22
)

// A trashSink is an IFileOperationProgressSink: PreDeleteItem refuses the
// deletes that would not go to the recycle bin, PostDeleteItem notes one
// that did not all the same and takes the path of the item the recycle
// bin holds; the other methods succeed.
type trashSink struct {
	vtbl    *[19]uintptr
	refs    int32
	refused bool   // a delete would not have gone to the recycle bin
	deleted bool   // an item was deleted for good all the same
	result  string // where the recycle bin put the first item it took
}

func initTrashSink() {
	cb := syscall.NewCallback
	ok := cb(func(this uintptr) uintptr { return sOK })
	ok3 := cb(func(this, a, b, c uintptr) uintptr { return sOK })
	ok5 := cb(func(this, a, b, c, d, e uintptr) uintptr { return sOK })
	ok6 := cb(func(this, a, b, c, d, e, f uintptr) uintptr { return sOK })
	trashSinkVtbl = [19]uintptr{
		cb(func(this, riid, out uintptr) uintptr {
			*(*uintptr)(native(out)) = 0
			if iid := *(*GUID)(native(riid)); iid != iidIUnknown && iid != iidIFileOperationProgressSink {
				return eNoInterface
			}
			*(*uintptr)(native(out)) = this
			trashSinks[this].refs++
			return sOK
		}),
		cb(func(this uintptr) uintptr { s := trashSinks[this]; s.refs++; return uintptr(s.refs) }),
		cb(func(this uintptr) uintptr { s := trashSinks[this]; s.refs--; return uintptr(s.refs) }),
		ok, // StartOperations
		cb(func(this, hr uintptr) uintptr { return sOK }), // FinishOperations
		ok3, // PreRenameItem
		ok5, // PostRenameItem
		cb(func(this, a, b, c, d uintptr) uintptr { return sOK }), // PreMoveItem
		ok6, // PostMoveItem
		cb(func(this, a, b, c, d uintptr) uintptr { return sOK }), // PreCopyItem
		ok6, // PostCopyItem
		// PreDeleteItem(flags, item)
		cb(func(this, flags, item uintptr) uintptr {
			if uint32(flags)&tsfDeleteRecycleIfPossible == 0 {
				trashSinks[this].refused = true
				return eAbort
			}
			return sOK
		}),
		// PostDeleteItem(flags, item, hrDelete, newlyCreated). A folder too
		// big for the recycle bin comes first with COPYENGINE_S_NOT_HANDLED
		// and no new item, though nothing is deleted yet: the shell then
		// deletes its items one by one.
		cb(func(this, flags, item, hrDelete, newlyCreated uintptr) uintptr {
			s := trashSinks[this]
			if !failed(hrDelete) && uint32(hrDelete) != copyEngineSNotHandled && newlyCreated == 0 {
				s.deleted = true
			}
			if newlyCreated != 0 && s.result == "" {
				var name uintptr
				if r := comCall(newlyCreated, siGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); !failed(r) {
					s.result = takeWstr(name)
				}
			}
			return sOK
		}),
		ok3, // PreNewItem
		cb(func(this, a, b, c, d, e, f, g uintptr) uintptr { return sOK }), // PostNewItem
		cb(func(this, total, done uintptr) uintptr { return sOK }),         // UpdateProgress
		ok, ok, ok, // ResetTimer, PauseTimer, ResumeTimer
	}
}

func (shell) TrashItem(path string) error {
	_, err := trashItem(path)
	return err
}

func (shell) TrashItemResult(path string) (string, error) {
	return trashItem(path)
}

// trashItem moves path to the recycle bin and returns where the bin put
// it: the $R item whose $I file names path, which the shell's progress
// sink reports as the delete's newly created item.
func trashItem(path string) (string, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		if capacity, ok := recycleBinCapacity(path); ok && uint64(info.Size()) > capacity<<20 {
			return "", fmt.Errorf("mygo: cannot move %s to the recycle bin: it is larger than the recycle bin (%d MB)", path, capacity)
		}
	}
	trashSinkOnce.Do(initTrashSink)
	var item uintptr
	if r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(u16(path))), 0, uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item))); failed(r) || item == 0 {
		return "", fmt.Errorf("mygo: cannot move %s to the recycle bin: %w", path, hresultError("SHCreateItemFromParsingName", r))
	}
	defer release(item)
	// The operation holds the sink until it is released, which the
	// deferred release below does before the sink leaves the map.
	sink := &trashSink{vtbl: &trashSinkVtbl, refs: 1}
	p := uintptr(unsafe.Pointer(sink))
	trashSinks[p] = sink
	defer delete(trashSinks, p)
	var op uintptr
	if r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, clsctxInprocServer, uintptr(unsafe.Pointer(&iidIFileOperation)), uintptr(unsafe.Pointer(&op))); failed(r) || op == 0 {
		return "", hresultError("CoCreateInstance(FileOperation)", r)
	}
	defer release(op)
	if r := comCall(op, foSetOperationFlags, fofNoUI|fofAllowUndo|fofxRecycleOnDelete|fofxAddUndoRecord); failed(r) {
		return "", hresultError("IFileOperation.SetOperationFlags", r)
	}
	if r := comCall(op, foDeleteItem, item, p); failed(r) {
		return "", hresultError("IFileOperation.DeleteItem", r)
	}
	r := comCall(op, foPerformOperations)
	var aborted int32
	comCall(op, foGetAnyOperationsAborted, uintptr(unsafe.Pointer(&aborted)))
	switch {
	case sink.refused:
		return "", fmt.Errorf("mygo: cannot move %s to the recycle bin: it would be deleted permanently", path)
	case sink.deleted:
		return "", fmt.Errorf("mygo: cannot move %s to the recycle bin: Windows deleted it permanently", path)
	case failed(r):
		return "", fmt.Errorf("mygo: cannot move %s to the recycle bin: %w", path, hresultError("IFileOperation.PerformOperations", r))
	case aborted != 0:
		return "", fmt.Errorf("mygo: cannot move %s to the recycle bin", path)
	}
	return sink.result, nil
}

// recycleBinKey returns the key, under HKEY_CURRENT_USER, of the settings
// of the recycle bin of path's volume. A network share has none.
func recycleBinKey(path string) (string, bool) {
	// The functions of the recycle bin's size are looked up here rather
	// than in package variables, which every app initializes, so that
	// apps that never trash a file do not link them.
	procGetVolumePathNameW := kernel32.NewProc("GetVolumePathNameW")
	procGetVolumeNameForVolumeMountPointW := kernel32.NewProc("GetVolumeNameForVolumeMountPointW")
	buf := make([]uint16, 32768)
	if r, _, _ := procGetVolumePathNameW.Call(uintptr(unsafe.Pointer(u16(path))), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
		return "", false
	}
	mount := syscall.UTF16ToString(buf)
	if r, _, _ := procGetVolumeNameForVolumeMountPointW.Call(uintptr(unsafe.Pointer(u16(mount))), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
		return "", false
	}
	volume := syscall.UTF16ToString(buf)
	if !strings.HasPrefix(volume, `\\?\Volume{`) || !strings.HasSuffix(volume, `}\`) {
		return "", false
	}
	return `Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket\Volume\` + strings.TrimSuffix(strings.TrimPrefix(volume, `\\?\Volume`), `\`), true
}

// recycleBinCapacity returns the size of the recycle bin of path's volume,
// in MB as Windows counts them (MiB): the largest file it takes.
func recycleBinCapacity(path string) (uint64, bool) {
	key, ok := recycleBinKey(path)
	if !ok {
		return 0, false
	}
	// The size in the recycle bin's properties.
	if data, ok := regGetBytes(hkeyCurrentUser, key, "MaxCapacity"); ok {
		if len(data) != 4 {
			return 0, false
		}
		return uint64(binary.LittleEndian.Uint32(data)), true
	}
	var total uint64
	if r, _, _ := kernel32.NewProc("GetDiskFreeSpaceExW").Call(uintptr(unsafe.Pointer(u16(filepath.Dir(path)))), 0, uintptr(unsafe.Pointer(&total)), 0); r == 0 {
		return 0, false
	}
	return defaultRecycleBinCapacity(total), true
}

// defaultRecycleBinCapacity is the size, in MB, of a recycle bin nobody has
// sized, on a volume of total bytes as the user's quota counts them: "ten
// percent of the first 40 GB of quota, and five percent of any quota above
// 40 GB", since Windows Vista (The Old New Thing, "What is the default size
// of the Recycle Bin, and how can an administrator control the size of the
// Recycle Bin?"). GitHub's Windows runners, whose volumes hold 160939618304
// and 274284392448 bytes, have recycle bins of 9722 and 15126 MB.
func defaultRecycleBinCapacity(total uint64) uint64 {
	first := min(total, 40<<30)
	return (first/10 + (total-first)/20) >> 20
}
