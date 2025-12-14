// Package installer provides Windows GUI installer functionality using native dialogs.
package installer

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
)

// MessageBox button types
const (
	MB_OK                = 0x00000000
	MB_OKCANCEL          = 0x00000001
	MB_ABORTRETRYIGNORE  = 0x00000002
	MB_YESNOCANCEL       = 0x00000003
	MB_YESNO             = 0x00000004
	MB_RETRYCANCEL       = 0x00000005
)

// MessageBox icon types
const (
	MB_ICONERROR       = 0x00000010
	MB_ICONQUESTION    = 0x00000020
	MB_ICONWARNING     = 0x00000030
	MB_ICONINFORMATION = 0x00000040
)

// MessageBox return values
const (
	IDOK     = 1
	IDCANCEL = 2
	IDABORT  = 3
	IDRETRY  = 4
	IDIGNORE = 5
	IDYES    = 6
	IDNO     = 7
)

// MessageBox displays a native Windows message box dialog.
func MessageBox(title, message string, flags uint32) int {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	messagePtr, _ := syscall.UTF16PtrFromString(message)

	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(flags),
	)

	return int(ret)
}

// ShowInfo displays an information message box with OK button.
func ShowInfo(title, message string) {
	MessageBox(title, message, MB_OK|MB_ICONINFORMATION)
}

// ShowError displays an error message box with OK button.
func ShowError(title, message string) {
	MessageBox(title, message, MB_OK|MB_ICONERROR)
}

// ShowWarning displays a warning message box with OK button.
func ShowWarning(title, message string) {
	MessageBox(title, message, MB_OK|MB_ICONWARNING)
}

// AskYesNo displays a Yes/No question dialog and returns true if Yes was clicked.
func AskYesNo(title, message string) bool {
	result := MessageBox(title, message, MB_YESNO|MB_ICONQUESTION)
	return result == IDYES
}

// AskYesNoCancel displays a Yes/No/Cancel question dialog.
// Returns 1 for Yes, 0 for No, -1 for Cancel.
func AskYesNoCancel(title, message string) int {
	result := MessageBox(title, message, MB_YESNOCANCEL|MB_ICONQUESTION)
	if result == IDYES {
		return 1
	}
	if result == IDNO {
		return 0
	}
	return -1
}

// AskOkCancel displays an OK/Cancel dialog and returns true if OK was clicked.
func AskOkCancel(title, message string) bool {
	result := MessageBox(title, message, MB_OKCANCEL|MB_ICONQUESTION)
	return result == IDOK
}

// ChoiceResult represents the user's choice from the install/uninstall dialog.
type ChoiceResult int

const (
	ChoiceCancel    ChoiceResult = 0
	ChoiceInstall   ChoiceResult = 1
	ChoiceUninstall ChoiceResult = 2
)

// AskInstallOrUninstall presents the user with install/uninstall options.
// Uses Yes for Install, No for Uninstall, Cancel to exit.
func AskInstallOrUninstall() ChoiceResult {
	result := MessageBox(
		"BgStatusService Setup",
		"Welcome to BgStatusService Setup!\n\n"+
			"This will install a Windows service that displays system information "+
			"on your login screen.\n\n"+
			"What would you like to do?\n\n"+
			"• Yes = Install / Upgrade\n"+
			"• No = Uninstall\n"+
			"• Cancel = Exit",
		MB_YESNOCANCEL|MB_ICONQUESTION,
	)

	if result == IDYES {
		return ChoiceInstall
	}
	if result == IDNO {
		return ChoiceUninstall
	}
	return ChoiceCancel
}

