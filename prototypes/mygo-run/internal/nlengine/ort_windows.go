//go:build windows

package nlengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// Verified OrtApi vtable slot indices for ONNX Runtime C API v17+ (ABI-stable across ORT 1.17..1.25+).
const (
	ortAPIVersion = 17

	slotGetErrorMessage                  = 2
	slotCreateEnv                        = 3
	slotCreateSession                    = 7
	slotRun                              = 9
	slotCreateSessionOptions             = 10
	slotSetSessionGraphOptimizationLevel = 23
	slotSetIntraOpNumThreads             = 24
	slotCreateTensorWithDataAsOrtValue   = 49
	slotGetTensorMutableData             = 51
	slotGetDimensionsCount               = 61
	slotGetDimensions                    = 62
	slotGetTensorTypeAndShape            = 65
	slotCreateCpuMemoryInfo              = 69
	slotReleaseEnv                       = 92
	slotReleaseStatus                    = 93
	slotReleaseMemoryInfo                = 94
	slotReleaseSession                   = 95
	slotReleaseValue                     = 96
	slotReleaseTensorTypeAndShapeInfo    = 99
	slotReleaseSessionOptions            = 100

	ortLoggingLevelError = 3
	ortGraphOptEnableAll = 99
	ortArenaAllocator    = 1
	ortMemTypeDefault    = 0
	onnxTensorTypeInt64  = 7
)

type ortAPIBase struct {
	getApi           uintptr
	getVersionString uintptr
}

// ORTSession wraps a live Windows onnxruntime.dll CPU inference session with zero CGO.
type ORTSession struct {
	mu          sync.Mutex
	dll         *syscall.LazyDLL
	apiSlots    *[128]uintptr
	env         uintptr
	session     uintptr
	memInfo     uintptr
	inputNames  []*byte
	outputName  *byte
	embedDim    int
	ortVersion  string
	dllPath     string
	closed      bool
}

// ResolveORTLibraryPath searches for onnxruntime.dll in clean upgrade/deployment order.
func ResolveORTLibraryPath(bundleDir string) string {
	var cands []string
	if env := strings.TrimSpace(os.Getenv("ASTERLAB_ORT_DLL")); env != "" {
		cands = append(cands, env)
	}
	if bundleDir != "" {
		cands = append(cands, filepath.Join(bundleDir, "onnxruntime.dll"))
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), "onnxruntime.dll"))
	}
	// Check common Python dev locations so local dev & `go test` work out of the box
	for _, pyRoot := range []string{
		`D:\programs\Python`,
		`C:\Python312`,
		`C:\Python311`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python", "Python312"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python", "Python311"),
	} {
		if pyRoot != "" {
			cands = append(cands, filepath.Join(pyRoot, "Lib", "site-packages", "onnxruntime", "capi", "onnxruntime.dll"))
		}
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "onnxruntime.dll" // fall back to Windows system DLL search path
}

// NewORTSession initializes onnxruntime.dll and loads modelPath for CPU inference.
func NewORTSession(bundleDir, modelPath string, inputNames []string, outputName string, embedDim int) (*ORTSession, error) {
	dllPath := ResolveORTLibraryPath(bundleDir)
	dll := syscall.NewLazyDLL(dllPath)
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("load onnxruntime.dll (%s): %w", dllPath, err)
	}
	getBaseProc := dll.NewProc("OrtGetApiBase")
	if err := getBaseProc.Find(); err != nil {
		return nil, fmt.Errorf("OrtGetApiBase not found in %s: %w", dllPath, err)
	}
	basePtr, _, _ := getBaseProc.Call()
	if basePtr == 0 {
		return nil, fmt.Errorf("OrtGetApiBase returned NULL")
	}
	base := (*ortAPIBase)(cPtr(basePtr))
	verPtr, _, _ := syscall.SyscallN(base.getVersionString)
	verStr := cStringToGo(verPtr)

	apiPtr, _, _ := syscall.SyscallN(base.getApi, uintptr(ortAPIVersion))
	if apiPtr == 0 {
		return nil, fmt.Errorf("onnxruntime %s does not support API v%d", verStr, ortAPIVersion)
	}
	slots := (*[128]uintptr)(cPtr(apiPtr))

	s := &ORTSession{
		dll:        dll,
		apiSlots:   slots,
		embedDim:   embedDim,
		ortVersion: verStr,
		dllPath:    dllPath,
	}

	// 1. CreateEnv
	logID, err := syscall.BytePtrFromString("asterlab_nl")
	if err != nil {
		return nil, err
	}
	if err := s.checkStatus(syscall.SyscallN(
		slots[slotCreateEnv],
		uintptr(ortLoggingLevelError),
		uintptr(unsafe.Pointer(logID)),
		uintptr(unsafe.Pointer(&s.env)),
	)); err != nil {
		return nil, fmt.Errorf("ORT CreateEnv: %w", err)
	}
	runtime.KeepAlive(logID)

	// 2. CreateSessionOptions (4 CPU threads, ORT_ENABLE_ALL)
	var sessOpts uintptr
	if err := s.checkStatus(syscall.SyscallN(
		slots[slotCreateSessionOptions],
		uintptr(unsafe.Pointer(&sessOpts)),
	)); err != nil {
		s.Close()
		return nil, fmt.Errorf("ORT CreateSessionOptions: %w", err)
	}
	defer func() {
		if sessOpts != 0 {
			syscall.SyscallN(slots[slotReleaseSessionOptions], sessOpts)
		}
	}()

	_ = s.checkStatus(syscall.SyscallN(slots[slotSetIntraOpNumThreads], sessOpts, uintptr(4)))
	_ = s.checkStatus(syscall.SyscallN(slots[slotSetSessionGraphOptimizationLevel], sessOpts, uintptr(ortGraphOptEnableAll)))

	// 3. CreateSession (Windows ORTCHAR_T is UTF-16)
	modelPathW, err := syscall.UTF16PtrFromString(modelPath)
	if err != nil {
		s.Close()
		return nil, err
	}
	if err := s.checkStatus(syscall.SyscallN(
		slots[slotCreateSession],
		s.env,
		uintptr(unsafe.Pointer(modelPathW)),
		sessOpts,
		uintptr(unsafe.Pointer(&s.session)),
	)); err != nil {
		s.Close()
		return nil, fmt.Errorf("ORT CreateSession(%s): %w", filepath.Base(modelPath), err)
	}
	runtime.KeepAlive(modelPathW)

	// 4. CreateCpuMemoryInfo
	if err := s.checkStatus(syscall.SyscallN(
		slots[slotCreateCpuMemoryInfo],
		uintptr(ortArenaAllocator),
		uintptr(ortMemTypeDefault),
		uintptr(unsafe.Pointer(&s.memInfo)),
	)); err != nil {
		s.Close()
		return nil, fmt.Errorf("ORT CreateCpuMemoryInfo: %w", err)
	}

	// Pre-allocate null-terminated C strings for input/output tensor names
	for _, nm := range inputNames {
		b, err := syscall.BytePtrFromString(nm)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.inputNames = append(s.inputNames, b)
	}
	outB, err := syscall.BytePtrFromString(outputName)
	if err != nil {
		s.Close()
		return nil, err
	}
	s.outputName = outB
	return s, nil
}

// ORTVersion returns the loaded onnxruntime.dll version string (e.g. "1.25.0").
func (s *ORTSession) ORTVersion() string {
	return s.ortVersion
}

// EncodeIDs runs a single-sequence forward pass [1, seqLen] -> [embedDim] float32.
func (s *ORTSession) EncodeIDs(inputIDs, attentionMask []int64) ([]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.session == 0 {
		return nil, fmt.Errorf("ORT session is closed")
	}
	seqLen := len(inputIDs)
	if seqLen == 0 || len(attentionMask) != seqLen {
		return nil, fmt.Errorf("invalid input lengths: ids=%d mask=%d", len(inputIDs), len(attentionMask))
	}

	shape := [2]int64{1, int64(seqLen)}
	byteLen := uintptr(seqLen * 8)

	var idsTensor, maskTensor uintptr
	if err := s.checkStatus(syscall.SyscallN(
		s.apiSlots[slotCreateTensorWithDataAsOrtValue],
		s.memInfo,
		uintptr(unsafe.Pointer(&inputIDs[0])),
		byteLen,
		uintptr(unsafe.Pointer(&shape[0])),
		uintptr(2),
		uintptr(onnxTensorTypeInt64),
		uintptr(unsafe.Pointer(&idsTensor)),
	)); err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer syscall.SyscallN(s.apiSlots[slotReleaseValue], idsTensor)

	if err := s.checkStatus(syscall.SyscallN(
		s.apiSlots[slotCreateTensorWithDataAsOrtValue],
		s.memInfo,
		uintptr(unsafe.Pointer(&attentionMask[0])),
		byteLen,
		uintptr(unsafe.Pointer(&shape[0])),
		uintptr(2),
		uintptr(onnxTensorTypeInt64),
		uintptr(unsafe.Pointer(&maskTensor)),
	)); err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer syscall.SyscallN(s.apiSlots[slotReleaseValue], maskTensor)

	inNamePtrs := [2]uintptr{
		uintptr(unsafe.Pointer(s.inputNames[0])),
		uintptr(unsafe.Pointer(s.inputNames[1])),
	}
	inTensors := [2]uintptr{idsTensor, maskTensor}
	outNamePtrs := [1]uintptr{uintptr(unsafe.Pointer(s.outputName))}
	var outTensors [1]uintptr

	if err := s.checkStatus(syscall.SyscallN(
		s.apiSlots[slotRun],
		s.session,
		0, // run_options = NULL
		uintptr(unsafe.Pointer(&inNamePtrs[0])),
		uintptr(unsafe.Pointer(&inTensors[0])),
		uintptr(2),
		uintptr(unsafe.Pointer(&outNamePtrs[0])),
		uintptr(1),
		uintptr(unsafe.Pointer(&outTensors[0])),
	)); err != nil {
		return nil, fmt.Errorf("ORT Run: %w", err)
	}
	defer func() {
		if outTensors[0] != 0 {
			syscall.SyscallN(s.apiSlots[slotReleaseValue], outTensors[0])
		}
	}()

	// Verify output shape & read float32 data
	var shapeInfo uintptr
	if err := s.checkStatus(syscall.SyscallN(
		s.apiSlots[slotGetTensorTypeAndShape],
		outTensors[0],
		uintptr(unsafe.Pointer(&shapeInfo)),
	)); err != nil {
		return nil, fmt.Errorf("ORT GetTensorTypeAndShape: %w", err)
	}
	defer syscall.SyscallN(s.apiSlots[slotReleaseTensorTypeAndShapeInfo], shapeInfo)

	var dimCount uintptr
	if err := s.checkStatus(syscall.SyscallN(
		s.apiSlots[slotGetDimensionsCount],
		shapeInfo,
		uintptr(unsafe.Pointer(&dimCount)),
	)); err != nil {
		return nil, err
	}
	dims := make([]int64, dimCount)
	if dimCount > 0 {
		if err := s.checkStatus(syscall.SyscallN(
			s.apiSlots[slotGetDimensions],
			shapeInfo,
			uintptr(unsafe.Pointer(&dims[0])),
			dimCount,
		)); err != nil {
			return nil, err
		}
	}
	totalFloats := 1
	for _, d := range dims {
		totalFloats *= int(d)
	}
	if totalFloats <= 0 {
		totalFloats = s.embedDim
	}

	var dataPtr unsafe.Pointer
	if err := s.checkStatus(syscall.SyscallN(
		s.apiSlots[slotGetTensorMutableData],
		outTensors[0],
		uintptr(unsafe.Pointer(&dataPtr)),
	)); err != nil {
		return nil, fmt.Errorf("ORT GetTensorMutableData: %w", err)
	}

	src := unsafe.Slice((*float32)(dataPtr), totalFloats)
	out := make([]float32, totalFloats)
	copy(out, src)

	runtime.KeepAlive(inputIDs)
	runtime.KeepAlive(attentionMask)
	runtime.KeepAlive(shape)
	runtime.KeepAlive(inNamePtrs)
	runtime.KeepAlive(inTensors)
	runtime.KeepAlive(outNamePtrs)
	return out, nil
}

// Close releases all native ONNX Runtime handles.
func (s *ORTSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.memInfo != 0 {
		syscall.SyscallN(s.apiSlots[slotReleaseMemoryInfo], s.memInfo)
		s.memInfo = 0
	}
	if s.session != 0 {
		syscall.SyscallN(s.apiSlots[slotReleaseSession], s.session)
		s.session = 0
	}
	if s.env != 0 {
		syscall.SyscallN(s.apiSlots[slotReleaseEnv], s.env)
		s.env = 0
	}
	return nil
}

func (s *ORTSession) checkStatus(status uintptr, _ uintptr, _ syscall.Errno) error {
	if status == 0 {
		return nil
	}
	msgPtr, _, _ := syscall.SyscallN(s.apiSlots[slotGetErrorMessage], status)
	msg := cStringToGo(msgPtr)
	syscall.SyscallN(s.apiSlots[slotReleaseStatus], status)
	return fmt.Errorf("%s", msg)
}

// cPtr converts a C-allocated non-Go memory address returned by Windows DLL syscalls to unsafe.Pointer.
func cPtr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func cStringToGo(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	base := cPtr(ptr)
	var buf []byte
	for i := 0; i < 4096; i++ {
		b := *(*byte)(unsafe.Add(base, i))
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}
