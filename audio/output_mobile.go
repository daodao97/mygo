//go:build ios || android

package audio

import (
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/mobile"
)

// Phones play through their hosts: AVAudioEngine on iOS, AudioTrack on Android.
// Writes block on this goroutine's thread, in the host, as the output fills.

type mobileDevice struct{}

// mobileFrames is how many frames go in a write: 10 ms.
const mobileFrames = sampleRate / 100

func openDevice(ahead time.Duration) (device, int, error) {
	if err := mobile.AudioOpen(sampleRate, int(ahead/time.Millisecond)); err != nil {
		return nil, 0, err
	}
	return mobileDevice{}, mobileFrames, nil
}

func (mobileDevice) write(samples []float32) error {
	if len(samples) == 0 {
		return nil
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(&samples[0])), len(samples)*4)
	return mobile.AudioWrite(b)
}

func (mobileDevice) close() { mobile.AudioClose() }
