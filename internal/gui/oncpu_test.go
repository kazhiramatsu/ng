package gui

import "testing"

// Devices as OpenGL (GL_RENDERER) and Vulkan name them: only CPU
// implementations, where moving painted rows is slow, are taken for one.
func TestOnCPU(t *testing.T) {
	cases := []struct {
		device string
		want   bool
	}{
		{"llvmpipe (LLVM 21.1.8, 128 bits)", true}, // Mesa, OpenGL and Vulkan
		{"softpipe", true},
		{"SwiftShader Device (Subzero)", true},
		{"Mesa X11 Software Rasterizer", true},
		{"Mesa Intel(R) UHD Graphics 620 (KBL GT2)", false},
		{"AMD Radeon RX 6600 (radeonsi, navi23, LLVM 17.0.6, DRM 3.54, 6.6.10)", false},
		{"NVIDIA GeForce RTX 3060/PCIe/SSE2", false},
		{"virgl (ANGLE (Apple, ANGLE Metal Renderer: Apple M5, Version 26.5.1))", false},
		{"", false},
	}
	for _, c := range cases {
		if got := onCPU(c.device); got != c.want {
			t.Errorf("onCPU(%q) = %v, want %v", c.device, got, c.want)
		}
	}
}
