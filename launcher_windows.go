//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// spawnHidden starts a process on Windows with the CREATE_NO_WINDOW flag
// so no console window appears.
func spawnHidden(nodePath, mainJS, workDir string, env []string) error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createProcess := kernel32.NewProc("CreateProcessW")

	commandLine := fmt.Sprintf(`"%s" "%s"`, nodePath, mainJS)
	cmdLinePtr, err := syscall.UTF16PtrFromString(commandLine)
	if err != nil {
		return fmt.Errorf("encode command line: %w", err)
	}

	workDirPtr, err := syscall.UTF16PtrFromString(workDir)
	if err != nil {
		return fmt.Errorf("encode work dir: %w", err)
	}

	// Build environment block
	envBlock, err := buildEnvBlock(env)
	if err != nil {
		return fmt.Errorf("build env block: %w", err)
	}

	const CREATE_NO_WINDOW = 0x08000000
	const CREATE_NEW_PROCESS_GROUP = 0x00000200

	si := &syscall.StartupInfo{}
	si.Cb = uint32(unsafe.Sizeof(*si))
	pi := &syscall.ProcessInformation{}

	r1, _, callErr := createProcess.Call(
		0, // lpApplicationName (null — use command line)
		uintptr(unsafe.Pointer(cmdLinePtr)),
		0, // lpProcessAttributes
		0, // lpThreadAttributes
		0, // bInheritHandles = false
		uintptr(CREATE_NO_WINDOW|CREATE_NEW_PROCESS_GROUP),
		uintptr(unsafe.Pointer(&envBlock[0])),
		uintptr(unsafe.Pointer(workDirPtr)),
		uintptr(unsafe.Pointer(si)),
		uintptr(unsafe.Pointer(pi)),
	)
	if r1 == 0 {
		return fmt.Errorf("CreateProcess failed: %w", callErr)
	}
	// Close handles we don't need
	_ = syscall.CloseHandle(pi.Thread)
	_ = syscall.CloseHandle(pi.Process)
	return nil
}

// buildEnvBlock converts a []string environment to the double-null-terminated
// block expected by CreateProcess.
func buildEnvBlock(env []string) ([]uint16, error) {
	var block []uint16
	for _, e := range env {
		encoded, err := syscall.UTF16FromString(e)
		if err != nil {
			return nil, err
		}
		block = append(block, encoded...)
	}
	block = append(block, 0) // final null terminator
	_ = os.Getenv            // suppress unused import
	return block, nil
}
