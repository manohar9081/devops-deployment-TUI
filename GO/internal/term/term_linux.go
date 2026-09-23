//go:build linux

package term

// tiocgwinsz is the TIOCGWINSZ ("get window size") ioctl request code on
// linux, from <asm-generic/ioctls.h>; it differs from the darwin value,
// hence the per-OS constant files.
const tiocgwinsz = 0x5413
