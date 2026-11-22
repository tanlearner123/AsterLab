//go:build !windows

package nlengine

import "fmt"

// ORTSession is a no-op stub on non-Windows platforms so VPS Linux test lanes compile cleanly.
type ORTSession struct{}

func ResolveORTLibraryPath(bundleDir string) string { return "" }

func NewORTSession(bundleDir, modelPath string, inputNames []string, outputName string, embedDim int) (*ORTSession, error) {
	return nil, fmt.Errorf("native ONNX Runtime DLL loader is currently enabled on Windows Gate")
}

func (s *ORTSession) ORTVersion() string { return "" }

func (s *ORTSession) EncodeIDs(inputIDs, attentionMask []int64) ([]float32, error) {
	return nil, fmt.Errorf("ORT unsupported on this platform")
}

func (s *ORTSession) Close() error { return nil }
