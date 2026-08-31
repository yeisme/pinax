//go:build windows

package pinaxclient

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func validateTokenFilePlatform(tokenPath string, _ os.FileInfo) error {
	descriptor, err := windows.GetNamedSecurityInfo(
		tokenPath,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil || descriptor == nil {
		return errTokenFilePolicy
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return errTokenFilePolicy
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !owner.Equals(user.User.Sid) {
		return errTokenFilePolicy
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errTokenFilePolicy
	}
	localSystem, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		return errTokenFilePolicy
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return errTokenFilePolicy
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errTokenFilePolicy
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(user.User.Sid) && !sid.Equals(localSystem) {
			return errTokenFilePolicy
		}
	}
	return nil
}

func validateTokenFilePathEntryPlatform(tokenPath string, _ os.FileInfo, _ bool) error {
	attributes, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(tokenPath))
	if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errTokenFilePolicy
	}
	return nil
}
