package credentials

import (
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	credNotFound            = syscall.Errno(1168)
)

var (
	credentialsDLL = syscall.NewLazyDLL("advapi32.dll")
	credSession    = credentialsDLL.NewProc("CredGetSessionTypes")
	credRead       = credentialsDLL.NewProc("CredReadW")
	credWrite      = credentialsDLL.NewProc("CredWriteW")
	credDelete     = credentialsDLL.NewProc("CredDeleteW")
	credFree       = credentialsDLL.NewProc("CredFree")
)

// windowsCredential mirrors CREDENTIALW, including pointer alignment on each
// supported Windows architecture. The AppRivet credential blob contains bytes.
type windowsCredential struct {
	Flags, Type     uint32
	Target, Comment *uint16
	LastWritten     syscall.Filetime
	BlobSize        uint32
	Blob            *byte
	Persist         uint32
	AttributeCount  uint32
	Attributes      unsafe.Pointer
	Alias, UserName *uint16
}

type windowsVault string

// NewVault returns the available operating-system credential store, or nil.
func NewVault() Vault {
	var persist [credTypeGeneric + 1]uint32
	ok, _, _ := credSession.Call(uintptr(len(persist)), uintptr(unsafe.Pointer(&persist[0])))
	if ok == 0 || persist[credTypeGeneric] < credPersistLocalMachine {
		return nil
	}
	return windowsVault("AppRivet/ProviderKey/")
}

func (v windowsVault) target(provider string) (*uint16, error) {
	if !ValidProvider(provider) {
		return nil, errProvider
	}
	return syscall.UTF16PtrFromString(string(v) + provider)
}

func (v windowsVault) Get(provider string) (string, error) {
	target, err := v.target(provider)
	if err != nil {
		return "", err
	}
	var credential *windowsCredential
	ok, _, callErr := credRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&credential)))
	if ok == 0 {
		if callErr == credNotFound {
			return "", nil
		}
		return "", callErr
	}
	defer credFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential.BlobSize > maxKeyBytes {
		return "", errKey
	}
	return string(unsafe.Slice(credential.Blob, credential.BlobSize)), nil
}

func (v windowsVault) Save(provider, key string) error {
	target, err := v.target(provider)
	if err != nil {
		return err
	}
	if !ValidKey(key) {
		return errKey
	}
	blob := []byte(key)
	defer clear(blob)
	credential := windowsCredential{Type: credTypeGeneric, Target: target,
		BlobSize: uint32(len(blob)), Blob: &blob[0], Persist: credPersistLocalMachine}
	ok, _, callErr := credWrite.Call(uintptr(unsafe.Pointer(&credential)), 0)
	if ok == 0 {
		return callErr
	}
	return nil
}

func (v windowsVault) Remove(provider string) error {
	target, err := v.target(provider)
	if err != nil {
		return err
	}
	ok, _, callErr := credDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	if ok == 0 && callErr != credNotFound {
		return callErr
	}
	return nil
}
