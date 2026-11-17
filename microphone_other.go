package mygo

const microphoneAvailable = false

func startMicrophone(func([]byte, error, bool)) (stop, cancel func()) {
	return func() {}, func() {}
}
