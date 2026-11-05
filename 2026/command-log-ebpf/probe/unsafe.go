package probe

import "unsafe"

// bytesPtr returns an unsafe.Pointer to the first element of b.
func bytesPtr(b []byte) unsafe.Pointer {
	return unsafe.Pointer(&b[0])
}

// bpfEventSize returns the size of the bpfEvent struct.
func bpfEventSize() uintptr {
	var e bpfEvent
	return unsafe.Sizeof(e)
}
