//go:build darwin

package term

// tiocgwinsz is the TIOCGWINSZ ("get window size") ioctl request code on
// darwin, from <sys/ioctl.h>; it differs from the linux value, hence the
// per-OS constant files.
const tiocgwinsz = 0x40087468
