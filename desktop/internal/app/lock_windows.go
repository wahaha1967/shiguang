//go:build windows

package app

import (
	"errors"

	"golang.org/x/sys/windows"
)

var singleInstanceMutex windows.Handle

func acquireSingleInstance() (func(), bool, error) {
	name, err := windows.UTF16PtrFromString(`Local\ShiguangStudyRoom`)
	if err != nil {
		return nil, false, err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return func() {}, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	singleInstanceMutex = handle
	return func() {
		if singleInstanceMutex != 0 {
			windows.CloseHandle(singleInstanceMutex)
			singleInstanceMutex = 0
		}
	}, false, nil
}

func notifyAlreadyRunning() {
	showMessage("拾光自习室已在运行，请使用已打开的窗口。", "拾光自习室")
}

func showError(caption, text string) {
	showMessage(text, caption)
}

func showMessage(text, caption string) {
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	captionPtr, err := windows.UTF16PtrFromString(caption)
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, textPtr, captionPtr, windows.MB_OK|windows.MB_ICONINFORMATION)
}
